package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaySuite(t *testing.T) {
	dir := t.TempDir()
	cass := filepath.Join(dir, "cassettes")
	if err := os.MkdirAll(cass, 0o755); err != nil {
		t.Fatal(err)
	}
	traj := `{
	  "case_id": "one",
	  "steps": [{"kind":"final","content":"hello"}],
	  "final_output": "hello",
	  "metrics": {}
	}`
	if err := os.WriteFile(filepath.Join(cass, "one.json"), []byte(traj), 0o644); err != nil {
		t.Fatal(err)
	}
	suite := `
name: toy
target: { kind: replay, dir: cassettes }
scorers:
  - kind: contains
  - kind: exact_match
cases:
  - id: one
    input: { user: hi }
    expected: { contains: hello, exact: hello }
`
	path := filepath.Join(dir, "suite.yaml")
	if err := os.WriteFile(path, []byte(suite), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Run(Options{
		SuitePath: path,
		Repeats:   2,
		StoreRoot: filepath.Join(dir, ".agenteval"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 0 || len(out.Summaries) != 1 || !out.Summaries[0].Passed {
		t.Fatalf("outcome %#v", out)
	}
	if out.Summaries[0].PassK != 1 {
		t.Fatalf("pass^k %v", out.Summaries[0].PassK)
	}
}
