package report

import (
	"fmt"

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
	for id, prev := range baseCases {
		next, ok := curCases[id]
		if !ok {
			d.Notes = append(d.Notes, fmt.Sprintf("note: case %s removed", id))
			continue
		}
		if prev.Passed && !next.Passed {
			msg := fmt.Sprintf("regression: case %s passed and now fails", id)
			if len(next.Failed) > 0 {
				msg += " (" + join(next.Failed) + ")"
			}
			d.Regressions = append(d.Regressions, msg)
		}
	}
	for id := range curCases {
		if _, ok := baseCases[id]; !ok {
			d.Notes = append(d.Notes, fmt.Sprintf("note: case %s is new", id))
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

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
