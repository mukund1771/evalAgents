package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mukund1771/evalAgents/internal/store"
)

// cassette writes a trajectory that calls lookup_order once and answers.
func cassette(t *testing.T, dir, id, tool, final string) {
	t.Helper()
	body := `{
	  "case_id": "` + id + `",
	  "steps": [
	    {"kind": "user", "content": "where is my order"},
	    {"kind": "assistant", "tool_calls": [{"name": "` + tool + `", "args": {"order_id": "1001"}}]},
	    {"kind": "tool_result", "content": "shipped", "call_id": "1"},
	    {"kind": "final", "content": "` + final + `"}
	  ],
	  "final_output": "` + final + `",
	  "metrics": {"tokens": 12}
	}`
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func suiteFile(t *testing.T, dir, name, cassettes string, ids ...string) string {
	t.Helper()
	body := `
name: ` + name + `
target: { kind: replay, dir: ` + cassettes + ` }
scorers:
  - kind: contains
  - kind: tool_trajectory_exact
  - kind: tools_succeeded
cases:
`
	for _, id := range ids {
		body += `  - id: ` + id + `
    input: { user: "where is order 1001" }
    expected:
      contains: shipped
      tools:
        - { name: lookup_order, args: { order_id: "1001" } }
`
	}
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestEndToEnd walks the documented workflow through the real exit codes:
// a green run, a regressed run, then diff as the CI gate.
func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".agenteval")

	// A passing suite, then a second suite where two of three cases break and a
	// fourth case is new. Several regressions plus several notes is what makes
	// the ordering of diff output observable.
	for _, id := range []string{"alpha", "beta", "gamma"} {
		cassette(t, filepath.Join(dir, "good"), id, "lookup_order", "shipped")
	}
	cassette(t, filepath.Join(dir, "bad"), "alpha", "lookup_order", "shipped")
	cassette(t, filepath.Join(dir, "bad"), "beta", "guess_order", "shipped")
	cassette(t, filepath.Join(dir, "bad"), "gamma", "guess_order", "shipped")
	cassette(t, filepath.Join(dir, "bad"), "delta", "lookup_order", "shipped")
	goodSuite := suiteFile(t, dir, "green", "good", "alpha", "beta", "gamma")
	badSuite := suiteFile(t, dir, "broken", "bad", "alpha", "beta", "gamma", "delta")

	var out, errb bytes.Buffer
	if code := run([]string{"run", goodSuite, "--root", root}, &out, &errb); code != 0 {
		t.Fatalf("green run exit %d\nstdout: %s\nstderr: %s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "pass^1 1.00") {
		t.Fatalf("green table = %s", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := run([]string{"run", badSuite, "--root", root}, &out, &errb); code != 1 {
		t.Fatalf("regressed run exit %d, want 1\nstdout: %s", code, out.String())
	}

	// Newest first, so [0] is the regressed run.
	mans, err := store.List(root)
	if err != nil || len(mans) != 2 {
		t.Fatalf("list = %#v err %v", mans, err)
	}
	bad, good := mans[0].ID, mans[1].ID

	out.Reset()
	errb.Reset()
	if code := run([]string{"list", "--root", root}, &out, &errb); code != 0 {
		t.Fatalf("list exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), good) || !strings.Contains(out.String(), bad) {
		t.Fatalf("list output = %s", out.String())
	}

	// The gate: a case that passed and now fails must exit 1.
	out.Reset()
	errb.Reset()
	if code := run([]string{"diff", good, bad, "--root", root}, &out, &errb); code != 1 {
		t.Fatalf("diff exit %d, want 1\nstdout: %s", code, out.String())
	}
	first := out.String()
	// Case order follows the suite file, so the report reads the way the suite does.
	wantOrder := []string{
		"regression: case beta passed and now fails",
		"regression: case gamma passed and now fails",
		"note: case delta is new",
	}
	at := 0
	for _, want := range wantOrder {
		i := strings.Index(first[at:], want)
		if i < 0 {
			t.Fatalf("diff output missing %q or out of order:\n%s", want, first)
		}
		at += i + len(want)
	}
	if strings.Contains(first, "case alpha") {
		t.Fatalf("alpha did not regress but appears: %s", first)
	}

	// Compare iterated maps once, which made this output reorder between runs.
	for i := 0; i < 20; i++ {
		var again bytes.Buffer
		if code := run([]string{"diff", good, bad, "--root", root}, &again, &errb); code != 1 {
			t.Fatalf("diff exit %d on repeat %d", code, i)
		}
		if again.String() != first {
			t.Fatalf("diff output is not stable\nfirst:\n%s\nrepeat %d:\n%s", first, i, again.String())
		}
	}

	// The reverse direction is an improvement, not a regression.
	out.Reset()
	errb.Reset()
	if code := run([]string{"diff", bad, good, "--root", root}, &out, &errb); code != 0 {
		t.Fatalf("reverse diff exit %d, want 0\nstdout: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "no regressions") {
		t.Fatalf("reverse diff = %s", out.String())
	}

	// show surfaces the failing scorer's reason and the step it came from.
	out.Reset()
	errb.Reset()
	if code := run([]string{"show", bad, "--case", "beta", "--root", root}, &out, &errb); code != 0 {
		t.Fatalf("show exit %d: %s", code, errb.String())
	}
	shown := out.String()
	if !strings.Contains(shown, "tool sequence mismatch") {
		t.Fatalf("show did not explain the failure: %s", shown)
	}
	if !strings.Contains(shown, "evidence step") {
		t.Fatalf("show did not print evidence: %s", shown)
	}
	if !strings.Contains(shown, "12 tokens") {
		t.Fatalf("show did not print target metrics: %s", shown)
	}
}

func TestBaselineGate(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".agenteval")
	cassette(t, filepath.Join(dir, "good"), "alpha", "lookup_order", "shipped")
	cassette(t, filepath.Join(dir, "good"), "beta", "lookup_order", "shipped")
	cassette(t, filepath.Join(dir, "bad"), "alpha", "lookup_order", "shipped")
	cassette(t, filepath.Join(dir, "bad"), "beta", "guess_order", "shipped")

	var out, errb bytes.Buffer
	if code := run([]string{"run", suiteFile(t, dir, "green", "good", "alpha", "beta"), "--root", root}, &out, &errb); code != 0 {
		t.Fatalf("green run exit %d: %s", code, errb.String())
	}
	mans, err := store.List(root)
	if err != nil || len(mans) != 1 {
		t.Fatalf("list = %#v err %v", mans, err)
	}

	// --baseline folds the comparison into the run, which is the CI one-liner.
	out.Reset()
	errb.Reset()
	code := run([]string{"run", suiteFile(t, dir, "broken", "bad", "alpha", "beta"), "--root", root, "--baseline", mans[0].ID}, &out, &errb)
	if code != 1 {
		t.Fatalf("baseline run exit %d, want 1\nstdout: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "regression: case beta") {
		t.Fatalf("baseline run did not report the regression: %s", out.String())
	}
}

func TestUsageAndUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 2 {
		t.Fatalf("no args exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "agenteval run") {
		t.Fatalf("no args did not print usage: %s", errb.String())
	}

	out.Reset()
	errb.Reset()
	if code := run([]string{"frobnicate"}, &out, &errb); code != 2 {
		t.Fatalf("unknown command exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown command "frobnicate"`) {
		t.Fatalf("unknown command stderr = %s", errb.String())
	}

	out.Reset()
	errb.Reset()
	if code := run([]string{"--help"}, &out, &errb); code != 0 {
		t.Fatalf("help exit %d, want 0", code)
	}
	if !strings.Contains(out.String(), "agenteval diff") {
		t.Fatalf("help output = %s", out.String())
	}
}

func TestMissingRunIsAnError(t *testing.T) {
	var out, errb bytes.Buffer
	root := filepath.Join(t.TempDir(), ".agenteval")
	if code := run([]string{"show", "nope", "--root", root}, &out, &errb); code != 2 {
		t.Fatalf("show missing run exit %d, want 2", code)
	}
	if code := run([]string{"diff", "nope", "alsonope", "--root", root}, &out, &errb); code != 2 {
		t.Fatalf("diff missing runs exit %d, want 2", code)
	}
}

func TestParseMatrix(t *testing.T) {
	key, vals, err := parseMatrix("model=a,b")
	if err != nil || key != "model" || len(vals) != 2 || vals[0] != "a" || vals[1] != "b" {
		t.Fatalf("key=%q vals=%#v err=%v", key, vals, err)
	}
	if _, _, err := parseMatrix("model"); err == nil {
		t.Fatal("expected an error for a value with no =")
	}
	if _, _, err := parseMatrix("model=a,"); err == nil {
		t.Fatal("expected an error for an empty value")
	}
	key, vals, err = parseMatrix("")
	if err != nil || key != "" || vals != nil {
		t.Fatalf("empty matrix = %q %#v %v", key, vals, err)
	}
}

func TestSplitArgsAllowsFlagsEitherSide(t *testing.T) {
	pos, flags := splitArgs([]string{"--root", "x", "suite.yaml", "--json"}, map[string]bool{"json": true})
	if len(pos) != 1 || pos[0] != "suite.yaml" {
		t.Fatalf("positionals = %#v", pos)
	}
	if len(flags) != 3 || flags[0] != "--root" || flags[1] != "x" || flags[2] != "--json" {
		t.Fatalf("flags = %#v", flags)
	}
}

// demo and init are the two commands a downloaded binary is used with first, so
// they are the two that must work with nothing on disk.
func TestDemoRunsWithNothingOnDisk(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	var out, errb bytes.Buffer
	if code := run([]string{"demo", "--root", root}, &out, &errb); code != 0 {
		t.Fatalf("demo exit %d, want 0\nstderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "regression") {
		t.Fatalf("demo did not reach the diff:\n%s", out.String())
	}
}

func TestInitWritesTheExampleAndRefusesToClobber(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myeval")
	var out, errb bytes.Buffer
	if code := run([]string{"init", dir}, &out, &errb); code != 0 {
		t.Fatalf("init exit %d: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "suite.yaml")); err != nil {
		t.Fatalf("suite.yaml not written: %v", err)
	}
	// The written suite has to actually run, or init handed over something broken.
	out.Reset()
	errb.Reset()
	root := filepath.Join(t.TempDir(), ".agenteval")
	if code := run([]string{"run", filepath.Join(dir, "suite.yaml"), "--root", root}, &out, &errb); code != 0 {
		t.Fatalf("the written suite exits %d: %s", code, errb.String())
	}

	// A second init must not overwrite edits someone has made.
	out.Reset()
	errb.Reset()
	if code := run([]string{"init", dir}, &out, &errb); code != 2 {
		t.Fatalf("init over a non-empty dir exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "not empty") {
		t.Fatalf("init stderr = %s", errb.String())
	}
}

func TestVersionIsReportable(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 {
		t.Fatalf("version exit %d", code)
	}
	if strings.TrimSpace(out.String()) != Version {
		t.Fatalf("version printed %q, want %q", out.String(), Version)
	}
	out.Reset()
	if code := run([]string{"--version"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != Version {
		t.Fatalf("--version exit %d out %q", code, out.String())
	}
}
