package demo

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evalagents "github.com/mukund1771/evalAgents"
	"github.com/mukund1771/evalAgents/internal/suite"
)

// The embedded suites are useless if a case has no cassette: the run would fail
// at replay time, in a binary someone just downloaded, on their first command.
// This checks the pairing inside the binary rather than on disk.
func TestEveryEmbeddedCaseHasACassette(t *testing.T) {
	dir := t.TempDir()
	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{SuitePath, RegressionPath} {
		su, err := suite.Load(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if len(su.Cases) == 0 {
			t.Fatalf("%s has no cases", rel)
		}
		for _, c := range su.Cases {
			path := filepath.Join(su.CassetteDir(), c.ID+".json")
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("%s case %s has no cassette: %v", rel, c.ID, err)
			}
		}
	}
}

// Extract must reproduce the embedded tree exactly, since init hands the result
// to someone to edit and run.
func TestExtractReproducesTheEmbeddedTree(t *testing.T) {
	dir := t.TempDir()
	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	var checked int
	err := fs.WalkDir(evalagents.Examples, Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		want, err := evalagents.Examples.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(Root, p)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return err
		}
		if string(got) != string(want) {
			t.Fatalf("%s differs on disk", rel)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 15 {
		t.Fatalf("only %d files extracted; the embed directive may have stopped matching", checked)
	}
}

// Run is the first thing a new user types. It must work with nothing on disk and
// must report the regression rather than hiding it.
func TestRunTellsTheWholeStory(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	var out, errb strings.Builder
	if code := Run(root, &out, &errb); code != 0 {
		t.Fatalf("demo exit %d, want 0\nstderr: %s", code, errb.String())
	}
	body := out.String()
	for _, want := range []string{
		"pass^1 1.00",   // the passing suite scored
		"pass^1 0.00",   // the regressed suite scored
		"10 regression", // and the diff named them
		"agenteval tui",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("demo output missing %q:\n%s", want, body)
		}
	}
	// Both runs must be browsable afterwards, which is the point of the last line.
	entries, err := os.ReadDir(filepath.Join(root, "runs"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("stored runs = %d err %v", len(entries), err)
	}
}
