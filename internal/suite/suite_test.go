package suite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const good = `
name: toy
target: { kind: replay, dir: cassettes }
scorers:
  - kind: contains
  - kind: steps_within
    max: 4
cases:
  - id: one
    tags: [a]
    input: { user: hi }
    expected: { contains: hello }
`

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "suite.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadGood(t *testing.T) {
	f, err := Load(write(t, good))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "toy" || len(f.Cases) != 1 || len(f.Scorers) != 2 {
		t.Fatalf("suite = %#v", f)
	}
	// YAML input becomes JSON for the target's stdin.
	if got := string(f.Cases[0].Input); got != `{"user":"hi"}` {
		t.Fatalf("input = %s", got)
	}
	if got := string(f.Cases[0].Expected); got != `{"contains":"hello"}` {
		t.Fatalf("expected = %s", got)
	}
	// Relative cassette dirs resolve against the suite file, not the working directory.
	if want := filepath.Join(f.Dir, "cassettes"); f.CassetteDir() != want {
		t.Fatalf("cassette dir = %s want %s", f.CassetteDir(), want)
	}
	if f.Timeout().String() != "30s" {
		t.Fatalf("default timeout = %s", f.Timeout())
	}
}

func TestLoadRejects(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"no name", "target: { kind: replay, dir: c }\nscorers: [{kind: contains}]\ncases: [{id: a}]\n", "suite name is required"},
		{"no cases", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: contains}]\n", "suite has no cases"},
		{"no scorers", "name: t\ntarget: { kind: replay, dir: c }\ncases: [{id: a}]\n", "suite has no scorers"},
		{"bad target", "name: t\ntarget: { kind: http }\nscorers: [{kind: contains}]\ncases: [{id: a}]\n", "target kind must be replay or cmd"},
		{"replay without dir", "name: t\ntarget: { kind: replay }\nscorers: [{kind: contains}]\ncases: [{id: a}]\n", "replay target requires dir"},
		{"cmd without command", "name: t\ntarget: { kind: cmd }\nscorers: [{kind: contains}]\ncases: [{id: a}]\n", "cmd target requires command"},
		{"bad timeout", "name: t\ntarget: { kind: cmd, command: [x], timeout: soon }\nscorers: [{kind: contains}]\ncases: [{id: a}]\n", "target timeout"},
		{"empty scorer kind", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{as: x}]\ncases: [{id: a}]\n", "scorer kind is required"},
		{"duplicate scorer", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: contains}, {kind: contains}]\ncases: [{id: a}]\n", `duplicate scorer name "contains"`},
		{"steps_within without max", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: steps_within}]\ncases: [{id: a}]\n", "steps_within requires max > 0"},
		{"bad judge metric", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: model_graded, metric: vibes}]\ncases: [{id: a}]\n", "model_graded metric must be"},
		{"missing case id", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: contains}]\ncases: [{input: {user: hi}}]\n", "case id is required"},
		{"duplicate case id", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: contains}]\ncases: [{id: a}, {id: a}]\n", `duplicate case id "a"`},
		{"not yaml", "name: [\n", "parse suite"},
		// A case id is also a cassette filename and a cell in every report.
		{"newline in case id", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: contains}]\ncases: [{id: \"a\\nb\"}]\n", "contains"},
		{"escape in case id", "name: t\ntarget: { kind: replay, dir: c }\nscorers: [{kind: contains}]\ncases: [{id: \"a\\u001bb\"}]\n", "contains"},
		{"newline in suite name", "name: \"a\\nb\"\ntarget: { kind: replay, dir: c }\nscorers: [{kind: contains}]\ncases: [{id: a}]\n", "suite name contains"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, tc.body))
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("expected an error for a missing suite file")
	}
}
