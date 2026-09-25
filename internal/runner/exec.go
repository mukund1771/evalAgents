package runner

import (
	"sync"

	"agenteval/internal/eval"
	"agenteval/internal/scorer"
	"agenteval/internal/store"
	"agenteval/internal/target"
)

func execute(cases []eval.Case, repeats, concurrency int, tgt target.Target, scorers []scorer.Scorer) []store.ResultRow {
	if concurrency < 1 {
		concurrency = 1
	}
	type job struct {
		cs     eval.Case
		repeat int
		index  int
	}
	var jobs []job
	for _, c := range cases {
		for rep := 0; rep < repeats; rep++ {
			jobs = append(jobs, job{cs: c, repeat: rep, index: len(jobs)})
		}
	}
	rows := make([]store.ResultRow, len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, jb := range jobs {
		jb := jb
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			rows[jb.index] = runOne(jb.cs, jb.repeat, tgt, scorers)
		}()
	}
	wg.Wait()
	return rows
}

func runOne(cs eval.Case, repeat int, tgt target.Target, scorers []scorer.Scorer) store.ResultRow {
	row := store.ResultRow{CaseID: cs.ID, Repeat: repeat}
	tr, err := tgt.Run(cs)
	if err != nil {
		row.Error = err.Error()
		row.Trajectory = tr
		return row
	}
	row.Trajectory = tr
	for _, s := range scorers {
		row.Scores = append(row.Scores, s.Score(cs, tr))
	}
	return row
}
