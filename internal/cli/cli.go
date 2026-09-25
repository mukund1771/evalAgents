// Package cli is the agenteval command line.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"agenteval/internal/report"
	"agenteval/internal/runner"
	"agenteval/internal/store"
)

// Run executes the CLI and returns a process exit code.
func Run(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return cmdRun(args[1:], stdout, stderr)
	case "diff":
		return cmdDiff(args[1:], stdout, stderr)
	case "list":
		return cmdList(args[1:], stdout, stderr)
	case "show":
		return cmdShow(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}

const usage = `agenteval — local agent evaluation

  agenteval run <suite.yaml> [--filter substr] [--repeats N] [--baseline id]
                             [--offline] [--record] [--json] [--junit path]
                             [--concurrency N] [--matrix key=a,b]
  agenteval diff <run_a> <run_b>
  agenteval list
  agenteval show <run_id> [--case id]

Runs are written to .agenteval/runs/<run_id>/. A nil score means skipped, not zero.
Exit 1 when a case fails or a baseline regresses.
`

func cmdRun(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	filter := fs.String("filter", "", "case id or tag substring")
	repeats := fs.Int("repeats", 1, "repeats per case; reports pass^k")
	baseline := fs.String("baseline", "", "compare against this run id and exit 1 on regression")
	offline := fs.Bool("offline", false, "replay cassettes; fail if one is missing")
	record := fs.Bool("record", false, "write cmd trajectories to the cassette directory")
	asJSON := fs.Bool("json", false, "print the summary as JSON")
	junit := fs.String("junit", "", "write JUnit XML to this path")
	conc := fs.Int("concurrency", 1, "cases to run at once")
	matrix := fs.String("matrix", "", "key=a,b env sweep for a cmd target")
	positionals, flagArgs := splitArgs(args, map[string]bool{
		"offline": true, "record": true, "json": true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: agenteval run <suite.yaml>")
		return 2
	}
	path := positionals[0]
	key, vals, err := parseMatrix(*matrix)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	out, err := runner.Run(runner.Options{
		SuitePath:   path,
		Filter:      *filter,
		Repeats:     *repeats,
		Baseline:    *baseline,
		Offline:     *offline,
		Record:      *record,
		Concurrency: *conc,
		MatrixKey:   key,
		MatrixVals:  vals,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		payload := any(out.Summaries)
		if len(out.Summaries) == 1 {
			payload = out.Summaries[0]
		}
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	} else {
		report.Table(stdout, out.Summaries...)
		report.MatrixTable(stdout, out.Summaries)
	}
	for _, d := range out.Regressions {
		fmt.Fprint(stdout, report.FormatDiff(d))
	}
	if *junit != "" {
		// One file for a single run. A matrix writes the last run, then a sibling per value.
		if len(out.Summaries) == 1 {
			if err := report.WriteJUnit(*junit, out.Summaries[0]); err != nil {
				fmt.Fprintln(stderr, err)
				return 2
			}
		} else {
			for _, sum := range out.Summaries {
				path := *junit
				if sum.Matrix != "" {
					path = strings.TrimSuffix(*junit, ".xml") + "-" + sanitizeFile(sum.Matrix) + ".xml"
				}
				if err := report.WriteJUnit(path, sum); err != nil {
					fmt.Fprintln(stderr, err)
					return 2
				}
			}
		}
	}
	return out.ExitCode
}

func cmdDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".agenteval", "run store root")
	positionals, flagArgs := splitArgs(args, nil)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positionals) != 2 {
		fmt.Fprintln(stderr, "usage: agenteval diff <run_a> <run_b>")
		return 2
	}
	base, err := store.LoadSummary(*root, positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	cur, err := store.LoadSummary(*root, positionals[1])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	d := report.Compare(base, cur)
	fmt.Fprint(stdout, report.FormatDiff(d))
	if len(d.Regressions) > 0 {
		return 1
	}
	return 0
}

func cmdList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".agenteval", "run store root")
	positionals, flagArgs := splitArgs(args, nil)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positionals) != 0 {
		fmt.Fprintln(stderr, "usage: agenteval list")
		return 2
	}
	mans, err := store.List(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(mans) == 0 {
		fmt.Fprintln(stdout, "no runs")
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUN\tSUITE\tWHEN\tTARGET")
	for _, m := range mans {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.ID, m.Suite, m.StartedAt.Format("2006-01-02T15:04:05Z"), m.Target)
	}
	tw.Flush()
	return 0
}

func cmdShow(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".agenteval", "run store root")
	caseID := fs.String("case", "", "show one case, including evidence")
	positionals, flagArgs := splitArgs(args, nil)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: agenteval show <run_id>")
		return 2
	}
	sum, err := store.LoadSummary(*root, positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *caseID == "" {
		report.Table(stdout, sum)
		return 0
	}
	rows, err := store.LoadResults(*root, positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	found := false
	for _, row := range rows {
		if row.CaseID != *caseID {
			continue
		}
		found = true
		fmt.Fprintf(stdout, "case %s repeat %d\n", row.CaseID, row.Repeat)
		if row.Error != "" {
			fmt.Fprintf(stdout, "error: %s\n", row.Error)
		}
		fmt.Fprintf(stdout, "final: %s\n", row.Trajectory.FinalOutput)
		for _, sc := range row.Scores {
			val := "-"
			if sc.Value != nil {
				val = fmt.Sprintf("%.2f", *sc.Value)
			}
			fmt.Fprintf(stdout, "  %s %s  %s\n", sc.Name, val, sc.Explanation)
			for _, ev := range sc.Evidence {
				fmt.Fprintf(stdout, "    evidence step %d [%d,%d)\n", ev.StepIndex, ev.Start, ev.End)
			}
		}
	}
	if !found {
		fmt.Fprintf(stderr, "case %s not in run %s\n", *caseID, positionals[0])
		return 2
	}
	return 0
}

// splitArgs lets flags appear before or after positional arguments.
// boolFlags lists names that do not take a value, without leading dashes.
func splitArgs(args []string, boolFlags map[string]bool) (positionals, flags []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			name, _, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
			if hasVal || boolFlags[name] {
				continue
			}
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, a)
	}
	return positionals, flags
}

func parseMatrix(s string) (string, []string, error) {
	if s == "" {
		return "", nil, nil
	}
	key, rest, ok := strings.Cut(s, "=")
	if !ok || key == "" || rest == "" {
		return "", nil, fmt.Errorf("--matrix must look like key=a,b")
	}
	var vals []string
	for _, v := range strings.Split(rest, ",") {
		if v == "" {
			return "", nil, fmt.Errorf("--matrix value is empty")
		}
		vals = append(vals, v)
	}
	return key, vals, nil
}

func sanitizeFile(s string) string {
	return strings.NewReplacer("=", "-", "/", "-", " ", "-").Replace(s)
}
