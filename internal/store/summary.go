package store

import (
	"sort"

	"agenteval/internal/scorer"
)

// PassK is the tau-bench estimator: C(c, k) / C(n, k), or 0 when c < k.
func PassK(successes, trials, k int) float64 {
	if k <= 0 {
		return 1
	}
	if successes < k || trials < k {
		return 0
	}
	return binom(successes, k) / binom(trials, k)
}

func binom(n, k int) float64 {
	if k < 0 || k > n {
		return 0
	}
	if k == 0 || k == n {
		return 1
	}
	if k > n-k {
		k = n - k
	}
	r := 1.0
	for i := 0; i < k; i++ {
		r = r * float64(n-i) / float64(i+1)
	}
	return r
}

// Summarize aggregates result rows into pass^k and per-scorer means.
// caseOrder controls the summary order. Rows for unknown ids are appended.
func Summarize(man Manifest, rows []ResultRow, caseOrder []string) Summary {
	type acc struct {
		successes int
		trials    int
		sums      map[string]float64
		counts    map[string]int
		passes    map[string]int
		skipped   map[string]int
		failed    map[string]bool
	}
	byID := map[string]*acc{}
	order := append([]string{}, caseOrder...)
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	for _, row := range rows {
		if !seen[row.CaseID] {
			order = append(order, row.CaseID)
			seen[row.CaseID] = true
		}
		a := byID[row.CaseID]
		if a == nil {
			a = &acc{
				sums:    map[string]float64{},
				counts:  map[string]int{},
				passes:  map[string]int{},
				skipped: map[string]int{},
				failed:  map[string]bool{},
			}
			byID[row.CaseID] = a
		}
		a.trials++
		if row.Error == "" && scorer.TrialPassed(row.Scores) {
			a.successes++
		}
		for _, sc := range row.Scores {
			if !sc.Applicable() {
				a.skipped[sc.Name]++
				continue
			}
			a.sums[sc.Name] += *sc.Value
			a.counts[sc.Name]++
			if sc.OK() {
				a.passes[sc.Name]++
			} else {
				a.failed[sc.Name] = true
			}
		}
	}

	sum := Summary{
		RunID:   man.ID,
		Suite:   man.Suite,
		Repeats: man.Repeats,
		Matrix:  man.Matrix,
		Passed:  true,
	}
	if sum.Repeats <= 0 {
		sum.Repeats = 1
	}
	scorerOrder := append([]string{}, man.Scorers...)
	agg := map[string]*ScorerAggregate{}
	for _, name := range scorerOrder {
		agg[name] = &ScorerAggregate{Name: name}
	}

	var p1, pk float64
	for _, id := range order {
		a := byID[id]
		if a == nil {
			continue
		}
		cs := CaseSummary{
			ID:        id,
			Successes: a.successes,
			Trials:    a.trials,
			Means:     map[string]float64{},
		}
		if a.trials > 0 {
			cs.Pass1 = PassK(a.successes, a.trials, 1)
			cs.PassK = PassK(a.successes, a.trials, a.trials)
			cs.Passed = a.successes == a.trials
		}
		if !cs.Passed {
			sum.Passed = false
		}
		for name, count := range a.counts {
			cs.Means[name] = a.sums[name] / float64(count)
		}
		for name := range a.failed {
			cs.Failed = append(cs.Failed, name)
		}
		sort.Strings(cs.Failed)
		sum.Cases = append(sum.Cases, cs)
		p1 += cs.Pass1
		pk += cs.PassK

		seenName := map[string]bool{}
		var names []string
		for name := range a.counts {
			if !seenName[name] {
				seenName[name] = true
				names = append(names, name)
			}
		}
		for name := range a.skipped {
			if !seenName[name] {
				seenName[name] = true
				names = append(names, name)
			}
		}
		// Sorted, so a scorer the manifest did not list still lands in a stable column.
		sort.Strings(names)
		for _, name := range names {
			if agg[name] == nil {
				agg[name] = &ScorerAggregate{Name: name}
				scorerOrder = append(scorerOrder, name)
			}
			g := agg[name]
			if a.counts[name] > 0 {
				g.Mean += a.sums[name]
				g.Count += a.counts[name]
				g.Passes += a.passes[name]
				g.HasValue = true
			}
			g.Skipped += a.skipped[name]
		}
	}
	if len(sum.Cases) > 0 {
		sum.Pass1 = p1 / float64(len(sum.Cases))
		sum.PassK = pk / float64(len(sum.Cases))
	}
	for _, name := range scorerOrder {
		g := agg[name]
		if g == nil {
			continue
		}
		if g.Count > 0 {
			g.Mean = g.Mean / float64(g.Count)
		}
		sum.Scorers = append(sum.Scorers, *g)
	}
	if len(sum.Cases) == 0 {
		sum.Passed = false
	}
	return sum
}
