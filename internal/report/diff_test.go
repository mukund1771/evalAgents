package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mukund1771/evalAgents/internal/store"
)

func TestCompareRegression(t *testing.T) {
	base := store.Summary{
		RunID:  "a",
		Passed: true,
		Cases:  []store.CaseSummary{{ID: "refund", Passed: true}},
		Scorers: []store.ScorerAggregate{
			{Name: "contains", Mean: 1, Count: 1},
		},
	}
	cur := store.Summary{
		RunID:  "b",
		Passed: false,
		Cases:  []store.CaseSummary{{ID: "refund", Passed: false, Failed: []string{"contains"}}},
		Scorers: []store.ScorerAggregate{
			{Name: "contains", Mean: 0, Count: 1},
		},
	}
	d := Compare(base, cur)
	if len(d.Regressions) < 2 {
		t.Fatalf("regressions = %#v", d.Regressions)
	}
	fresh := Compare(base, base)
	if len(fresh.Regressions) != 0 {
		t.Fatalf("clean diff = %#v", fresh)
	}
	added := Compare(base, store.Summary{
		RunID: "c",
		Cases: []store.CaseSummary{{ID: "refund", Passed: true}, {ID: "new", Passed: true}},
		Scorers: []store.ScorerAggregate{
			{Name: "contains", Mean: 1, Count: 2},
		},
	})
	if len(added.Regressions) != 0 || len(added.Notes) != 1 {
		t.Fatalf("new case = %#v", added)
	}
}

// A case no scorer applied to was reported as a hard failure, which in a CI UI
// looks identical to an assertion that ran and was wrong.
func TestJUnitMarksAnUngradedCaseSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.xml")
	sum := store.Summary{
		Suite: "s",
		Cases: []store.CaseSummary{
			{ID: "graded", Passed: true, Means: map[string]float64{"contains": 1}},
			{ID: "failed", Passed: false, Means: map[string]float64{"contains": 0}, Failed: []string{"contains"}},
			{ID: "ungraded", Passed: false}, // every scorer skipped, so Means is empty
		},
	}
	if err := WriteJUnit(path, sum); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, `failures="1"`) {
		t.Fatalf("want one failure:\n%s", got)
	}
	if !strings.Contains(got, `skipped="1"`) {
		t.Fatalf("want one skipped:\n%s", got)
	}
	if !strings.Contains(got, "no scorer applied") {
		t.Fatalf("the skip should say why:\n%s", got)
	}
}
