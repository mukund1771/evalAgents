package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mukund1771/evalAgents/internal/store"
)

// Every disk access in this package is in this file, and all of it is a read.
// That is the whole basis for calling the TUI read-only, so it is worth being
// able to check the claim by reading sixty lines.
//
// The commands below capture only their string arguments, never the model or a
// pointer to it. They return data and mutate nothing, which is the entire
// concurrency story: there is no shared state for the race detector to find.

// runRow is one row of the runs list: the manifest, and the summary that carries
// pass^1. err is set when this run's summary would not load, and the row is kept
// anyway so the list never silently loses a run.
type runRow struct {
	man store.Manifest
	sum store.Summary
	err string
}

// runsMsg carries every manifest and summary. Summaries hold no trajectories, so
// reading all of them up front is cheaper than a spinner and a loading state.
type runsMsg struct {
	rows []runRow
	err  error
}

// resultsMsg carries results.jsonl for one run. The id travels with the message
// so a late reply for a run the user has already left is dropped rather than
// filed against whatever they are looking at now.
type resultsMsg struct {
	runID string
	rows  []store.ResultRow
	err   error
}

func loadRuns(root string) tea.Cmd {
	return func() tea.Msg {
		mans, err := store.List(root)
		if err != nil {
			return runsMsg{err: err}
		}
		rows := make([]runRow, 0, len(mans))
		for _, m := range mans {
			row := runRow{man: m}
			sum, err := store.LoadSummary(root, m.ID)
			if err != nil {
				row.err = err.Error()
			} else {
				row.sum = sum
			}
			rows = append(rows, row)
		}
		return runsMsg{rows: rows}
	}
}

// loadResults is lazy because results.jsonl embeds full trajectories, and
// store.LoadResults raises the scanner limit to 10MB per line for that reason.
func loadResults(root, id string) tea.Cmd {
	return func() tea.Msg {
		rows, err := store.LoadResults(root, id)
		return resultsMsg{runID: id, rows: rows, err: err}
	}
}
