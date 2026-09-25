// Package runner executes a suite and writes an immutable run.
package runner

import (
	"fmt"
	"strings"
	"time"

	"agenteval/internal/eval"
	"agenteval/internal/judge"
	"agenteval/internal/report"
	"agenteval/internal/scorer"
	"agenteval/internal/store"
	"agenteval/internal/suite"
	"agenteval/internal/target"
)

// Options controls one invocation of run.
type Options struct {
	SuitePath   string
	Filter      string
	Repeats     int
	Baseline    string
	Offline     bool
	Record      bool
	Concurrency int
	MatrixKey   string
	MatrixVals  []string
	StoreRoot   string
	Now         func() time.Time
}

// Outcome is what the CLI prints and how it exits.
type Outcome struct {
	Summaries   []store.Summary
	Regressions []report.Diff
	ExitCode    int
}

// Run loads the suite, executes every matrix value, and stores each run.
func Run(opt Options) (Outcome, error) {
	if opt.Repeats <= 0 {
		opt.Repeats = 1
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = 1
	}
	if opt.StoreRoot == "" {
		opt.StoreRoot = ".agenteval"
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Offline && opt.Record {
		return Outcome{}, fmt.Errorf("--offline and --record cannot be combined")
	}
	su, err := suite.Load(opt.SuitePath)
	if err != nil {
		return Outcome{}, err
	}
	cases := filterCases(su.Cases, opt.Filter)
	if len(cases) == 0 {
		return Outcome{}, fmt.Errorf("no cases matched filter %q", opt.Filter)
	}
	values := opt.MatrixVals
	if len(values) == 0 {
		values = []string{""}
	}
	j := judge.FromEnv()
	scorers, err := scorer.Build(su.Scorers, j, j.Enabled())
	if err != nil {
		return Outcome{}, err
	}
	names := make([]string, len(scorers))
	for i, s := range scorers {
		names[i] = s.Name()
	}
	order := make([]string, len(cases))
	for i, c := range cases {
		order[i] = c.ID
	}

	var out Outcome
	out.ExitCode = 0
	for _, val := range values {
		matrix := ""
		if opt.MatrixKey != "" && val != "" {
			matrix = opt.MatrixKey + "=" + val
		}
		id, err := store.NewRunID(opt.Now())
		if err != nil {
			return out, err
		}
		// Distinct ids when the clock does not advance between matrix cells.
		if val != "" {
			id = id + "-" + sanitize(val)
		}
		run, err := store.Create(opt.StoreRoot, id)
		if err != nil {
			return out, err
		}
		tgt, err := buildTarget(su, opt, val)
		if err != nil {
			return out, err
		}
		rows := execute(cases, opt.Repeats, opt.Concurrency, tgt, scorers)
		man := store.Manifest{
			ID:        id,
			Suite:     su.Name,
			StartedAt: opt.Now().UTC(),
			Target:    su.Target.Kind,
			Scorers:   names,
			Repeats:   opt.Repeats,
			Filter:    opt.Filter,
			Matrix:    matrix,
			Command:   su.Target.Command,
		}
		sum := store.Summarize(man, rows, order)
		if err := run.WriteManifest(man); err != nil {
			return out, err
		}
		if err := run.WriteResults(rows); err != nil {
			return out, err
		}
		if err := run.WriteSummary(sum); err != nil {
			return out, err
		}
		out.Summaries = append(out.Summaries, sum)
		if !sum.Passed {
			out.ExitCode = 1
		}
		if opt.Baseline != "" {
			base, err := store.LoadSummary(opt.StoreRoot, opt.Baseline)
			if err != nil {
				return out, fmt.Errorf("baseline %s: %w", opt.Baseline, err)
			}
			d := report.Compare(base, sum)
			out.Regressions = append(out.Regressions, d)
			if len(d.Regressions) > 0 {
				out.ExitCode = 1
			}
		}
	}
	return out, nil
}

func buildTarget(su *suite.File, opt Options, matrixVal string) (target.Target, error) {
	switch su.Target.Kind {
	case "replay":
		if opt.Record {
			return nil, fmt.Errorf("--record requires a cmd target")
		}
		return target.Replay{Dir: su.CassetteDir()}, nil
	case "cmd":
		env := []string{}
		if opt.MatrixKey != "" && matrixVal != "" {
			env = target.Environ(opt.MatrixKey, matrixVal)
		}
		return target.Cmd{
			Command:     su.Target.Command,
			Timeout:     su.Timeout(),
			WorkDir:     su.Dir,
			Env:         env,
			CassetteDir: su.CassetteDir(),
			Offline:     opt.Offline,
			Record:      opt.Record,
		}, nil
	default:
		return nil, fmt.Errorf("unknown target %q", su.Target.Kind)
	}
}

func filterCases(cases []eval.Case, filter string) []eval.Case {
	if filter == "" {
		return cases
	}
	var out []eval.Case
	for _, c := range cases {
		if strings.Contains(c.ID, filter) {
			out = append(out, c)
			continue
		}
		for _, tag := range c.Tags {
			if strings.Contains(tag, filter) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func sanitize(v string) string {
	b := make([]byte, 0, len(v))
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b = append(b, c)
		} else {
			b = append(b, '-')
		}
	}
	if len(b) == 0 {
		return "x"
	}
	return string(b)
}
