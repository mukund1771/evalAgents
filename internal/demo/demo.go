// Package demo materialises the embedded example suite and runs it.
//
// A downloaded binary has no repository next to it, so without this there is
// nothing to evaluate and the first command a new user types fails. Extract
// writes the example to disk and the eval path then reads it like any other
// suite: no fs.FS is threaded through suite.Load or the targets, because that
// would spread an abstraction across the core packages to save one temp dir.
package demo

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	evalagents "github.com/mukund1771/evalAgents"
	"github.com/mukund1771/evalAgents/internal/report"
	"github.com/mukund1771/evalAgents/internal/runner"
	"github.com/mukund1771/evalAgents/internal/store"
)

// Root is the directory inside the embedded filesystem holding the example.
const Root = "examples/support"

// SuitePath and RegressionPath are the two suites Run uses, relative to the
// directory Extract was given.
const (
	SuitePath      = "suite.yaml"
	RegressionPath = "regression/suite.yaml"
)

// Extract writes the embedded example into dir, creating it if needed.
// Existing files are overwritten; Extract is for a directory you own.
func Extract(dir string) error {
	return fs.WalkDir(evalagents.Examples, Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(Root, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := evalagents.Examples.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
}

// Run is the guided tour: score a passing suite, score the regressed variant,
// then diff the two. It demonstrates the gate rather than describing it.
//
// The return is 0 when the tour completed and 2 when something went wrong.
// It is deliberately not 1 even though two of the three steps exit 1 on their
// own: those non-zero exits are the point being demonstrated, and a demo that
// failed its own shell would read as a broken binary.
func Run(storeRoot string, stdout, stderr io.Writer) int {
	dir, err := os.MkdirTemp("", "agenteval-demo-")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	defer os.RemoveAll(dir)
	if err := Extract(dir); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	fmt.Fprintf(stdout, "agenteval demo - no files, no API key, no network\n"+
		"The example is embedded in this binary and unpacked to a temp directory.\n\n")

	fmt.Fprintln(stdout, "1/3  ten recorded trajectories for a support agent")
	good, code := runSuite(storeRoot, filepath.Join(dir, SuitePath), stdout, stderr)
	if code != 0 {
		return code
	}

	fmt.Fprintln(stdout, "\n2/3  the same agent after a regression, on three of those cases")
	bad, code := runSuite(storeRoot, filepath.Join(dir, RegressionPath), stdout, stderr)
	if code != 0 {
		return code
	}
	fmt.Fprintln(stdout, "     that run exits 1, which is what fails a CI job")

	fmt.Fprintln(stdout, "\n3/3  what changed between the two runs")
	d := report.Compare(good, bad)
	fmt.Fprint(stdout, report.FormatDiff(d))
	fmt.Fprintln(stdout, "     diff exits 1 too, so a merge can be blocked on it")

	fmt.Fprintf(stdout, "\nBoth runs are stored under %s. From here:\n"+
		"  agenteval tui                 browse them\n"+
		"  agenteval diff %s %s\n"+
		"  agenteval init myeval         write the example out and edit it\n",
		storeRoot, good.RunID, bad.RunID)
	return 0
}

// runSuite executes one suite and prints its table. The suite's own exit code is
// discarded: the caller is narrating, not gating.
func runSuite(storeRoot, path string, stdout, stderr io.Writer) (store.Summary, int) {
	out, err := runner.Run(runner.Options{SuitePath: path, StoreRoot: storeRoot})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return store.Summary{}, 2
	}
	if len(out.Summaries) != 1 {
		fmt.Fprintf(stderr, "expected one run, got %d\n", len(out.Summaries))
		return store.Summary{}, 2
	}
	report.Table(stdout, out.Summaries[0])
	return out.Summaries[0], 0
}
