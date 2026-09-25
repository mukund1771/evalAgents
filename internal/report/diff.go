package report

import (
	"fmt"
	"strings"

	"agenteval/internal/store"
)

const meanEpsilon = 1e-9

// Diff is the comparison of a baseline run (A) to a candidate run (B).
type Diff struct {
	Baseline    string
	Current     string
	Regressions []string
	Notes       []string
}

// Compare treats base as the previous run and cur as the candidate.
// A case that used to pass and now fails is a regression.
// A scorer mean that drops is a regression.
// Cases only in the candidate are notes, not regressions.
func Compare(base, cur store.Summary) Diff {
	d := Diff{Baseline: base.RunID, Current: cur.RunID}
	baseCases := map[string]store.CaseSummary{}
	for _, c := range base.Cases {
		baseCases[c.ID] = c
	}
	curCases := map[string]store.CaseSummary{}
	for _, c := range cur.Cases {
		curCases[c.ID] = c
	}
	// Iterate the stored slices, not the lookup maps: diff output feeds CI logs
	// and a golden test, so the order has to be the same on every run.
	for _, prev := range base.Cases {
		next, ok := curCases[prev.ID]
		if !ok {
			d.Notes = append(d.Notes, fmt.Sprintf("note: case %s removed", prev.ID))
			continue
		}
		if prev.Passed && !next.Passed {
			msg := fmt.Sprintf("regression: case %s passed and now fails", prev.ID)
			if len(next.Failed) > 0 {
				msg += " (" + strings.Join(next.Failed, ", ") + ")"
			}
			d.Regressions = append(d.Regressions, msg)
		}
	}
	for _, next := range cur.Cases {
		if _, ok := baseCases[next.ID]; !ok {
			d.Notes = append(d.Notes, fmt.Sprintf("note: case %s is new", next.ID))
		}
	}
	baseSc := map[string]store.ScorerAggregate{}
	for _, s := range base.Scorers {
		baseSc[s.Name] = s
	}
	for _, s := range cur.Scorers {
		prev, ok := baseSc[s.Name]
		if !ok || prev.Count == 0 || s.Count == 0 {
			continue
		}
		if s.Mean < prev.Mean-meanEpsilon {
			d.Regressions = append(d.Regressions, fmt.Sprintf("regression: %s mean %.4f -> %.4f", s.Name, prev.Mean, s.Mean))
		}
	}
	return d
}
