package report

import (
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
