// Package suite loads a YAML eval suite from disk.
package suite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode"

	"github.com/mukund1771/evalAgents/internal/eval"

	"gopkg.in/yaml.v3"
)

// ScorerSpec is one scorer entry in the suite file.
type ScorerSpec struct {
	Kind       string   `yaml:"kind"`
	As         string   `yaml:"as"`
	Max        int      `yaml:"max"`
	MaxRepeats int      `yaml:"max_repeats"`
	Min        *float64 `yaml:"min"`
	Metric     string   `yaml:"metric"`
}

// Name is the score name written into results. As overrides Kind.
func (s ScorerSpec) Name() string {
	if s.As != "" {
		return s.As
	}
	return s.Kind
}

// MinScore is the pass threshold. The default is 1.
func (s ScorerSpec) MinScore() float64 {
	if s.Min == nil {
		return 1
	}
	return *s.Min
}

// TargetSpec selects replay or cmd.
type TargetSpec struct {
	Kind      string   `yaml:"kind"`
	Command   []string `yaml:"command"`
	Timeout   string   `yaml:"timeout"`
	Dir       string   `yaml:"dir"`
	Cassettes string   `yaml:"cassettes"`
}

// File is a parsed suite. Dir is the directory containing the YAML file.
type File struct {
	Name    string       `yaml:"name"`
	Target  TargetSpec   `yaml:"target"`
	Scorers []ScorerSpec `yaml:"scorers"`
	Cases   []eval.Case  `yaml:"-"`
	Dir     string       `yaml:"-"`
	Path    string       `yaml:"-"`
}

type rawFile struct {
	Name    string       `yaml:"name"`
	Target  TargetSpec   `yaml:"target"`
	Scorers []ScorerSpec `yaml:"scorers"`
	Cases   []rawCase    `yaml:"cases"`
}

type rawCase struct {
	ID       string   `yaml:"id"`
	Input    any      `yaml:"input"`
	Expected any      `yaml:"expected"`
	Tags     []string `yaml:"tags"`
}

// Load reads a suite YAML file and resolves nothing yet: relative target paths
// stay relative to File.Dir until the runner builds the target.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw rawFile
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse suite: %w", err)
	}
	if raw.Name == "" {
		return nil, fmt.Errorf("suite name is required")
	}
	if bad, ok := unprintable(raw.Name); ok {
		return nil, fmt.Errorf("suite name contains %q", bad)
	}
	if len(raw.Cases) == 0 {
		return nil, fmt.Errorf("suite has no cases")
	}
	// Without a scorer nothing applies, and every case would silently fail.
	if len(raw.Scorers) == 0 {
		return nil, fmt.Errorf("suite has no scorers")
	}
	if err := validateTarget(raw.Target); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, sc := range raw.Scorers {
		if sc.Kind == "" {
			return nil, fmt.Errorf("scorer kind is required")
		}
		name := sc.Name()
		if seen[name] {
			return nil, fmt.Errorf("duplicate scorer name %q", name)
		}
		seen[name] = true
		if sc.Kind == "steps_within" && sc.Max <= 0 {
			return nil, fmt.Errorf("steps_within requires max > 0")
		}
		if sc.Kind == "model_graded" && sc.Metric != "" && sc.Metric != "task_completion" && sc.Metric != "faithfulness" {
			return nil, fmt.Errorf("model_graded metric must be task_completion or faithfulness")
		}
	}
	cases := make([]eval.Case, 0, len(raw.Cases))
	ids := map[string]bool{}
	for _, c := range raw.Cases {
		if c.ID == "" {
			return nil, fmt.Errorf("case id is required")
		}
		if ids[c.ID] {
			return nil, fmt.Errorf("duplicate case id %q", c.ID)
		}
		// A case id is also a cassette filename and a cell in every report, so
		// it has to be one line of printable text. Rejecting it here is cheaper
		// than every consumer defending itself.
		if bad, ok := unprintable(c.ID); ok {
			return nil, fmt.Errorf("case id %q contains %q", c.ID, bad)
		}
		ids[c.ID] = true
		in, err := toJSON(c.Input)
		if err != nil {
			return nil, fmt.Errorf("case %s input: %w", c.ID, err)
		}
		exp, err := toJSON(c.Expected)
		if err != nil {
			return nil, fmt.Errorf("case %s expected: %w", c.ID, err)
		}
		cases = append(cases, eval.Case{ID: c.ID, Input: in, Expected: exp, Tags: c.Tags})
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &File{
		Name:    raw.Name,
		Target:  raw.Target,
		Scorers: raw.Scorers,
		Cases:   cases,
		Dir:     filepath.Dir(abs),
		Path:    abs,
	}, nil
}

func validateTarget(t TargetSpec) error {
	switch t.Kind {
	case "replay":
		if t.Dir == "" && t.Cassettes == "" {
			return fmt.Errorf("replay target requires dir")
		}
	case "cmd":
		if len(t.Command) == 0 {
			return fmt.Errorf("cmd target requires command")
		}
		if t.Timeout != "" {
			if _, err := time.ParseDuration(t.Timeout); err != nil {
				return fmt.Errorf("target timeout: %w", err)
			}
		}
	default:
		return fmt.Errorf("target kind must be replay or cmd, got %q", t.Kind)
	}
	return nil
}

// CassetteDir is the directory of trajectory JSON files, relative to the suite
// file when the configured path is relative.
func (f *File) CassetteDir() string {
	dir := f.Target.Dir
	if f.Target.Kind == "cmd" && f.Target.Cassettes != "" {
		dir = f.Target.Cassettes
	}
	if dir == "" {
		dir = f.Target.Cassettes
	}
	if dir == "" {
		return ""
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(f.Dir, dir)
}

// Timeout returns the cmd timeout, or 30s when unset.
func (f *File) Timeout() time.Duration {
	if f.Target.Timeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(f.Target.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

func toJSON(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// unprintable reports the first control or format rune in s, if any.
func unprintable(s string) (rune, bool) {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return r, true
		}
	}
	return 0, false
}
