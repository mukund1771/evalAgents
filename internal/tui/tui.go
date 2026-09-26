// Package tui is a read-only browser over the run store.
//
// It renders what the CLI prints and recomputes nothing: the run-detail table is
// report.Table's own output and the diff is report.FormatDiff's, so the two can
// never disagree about a number. Nothing here writes to disk, launches a run, or
// edits a suite. That restraint is the point — it keeps `list` and `show`
// navigable without putting the runner behind an interactive surface.
package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mukund1771/evalAgents/internal/report"
	"github.com/mukund1771/evalAgents/internal/store"
)

type view int

const (
	viewRuns view = iota
	viewRunDetail
	viewCaseDetail
	viewDiff
)

// chrome is the number of lines outside the viewport: title, status, footer.
// It is constant so toggling help cannot resize the scrolling area underneath.
const chrome = 3

// model is the whole TUI state.
//
// One struct with a view enum rather than nested tea.Models: all four views share
// the same root, rows, marks, viewport and size, none of them owns independent
// input state, and "esc goes up a level" is a four-line switch here versus a
// parent-pointer protocol there. A tea.Model wrapping a cursor integer would be
// ceremony, and this repo was just cleaned of that.
type model struct {
	root string

	view   view
	vp     viewport.Model
	st     styles
	w, h   int
	help   bool
	loaded bool

	// err is a whole-operation failure; status is a one-shot message that
	// survives exactly one render.
	err    string
	status string

	rows       []runRow
	runCursor  int
	caseCursor int
	repeat     int
	marked     []string
	diff       report.Diff

	results   map[string][]store.ResultRow
	resultErr map[string]string
}

func newModel(root string, st styles) model {
	m := model{
		root:      root,
		st:        st,
		w:         80,
		h:         24,
		results:   map[string][]store.ResultRow{},
		resultErr: map[string]string{},
	}
	m.vp = viewport.New(m.w, m.h-chrome)
	// The viewport's default keymap binds space and f to page-down, d to
	// half-page-down and u to half-page-up. This TUI needs space for marking and
	// d for diff, so the keymap is emptied and the viewport is driven by method
	// call only. That also makes the switch in key() the single exhaustive
	// account of what every key does.
	m.vp.KeyMap = viewport.KeyMap{}
	return m
}

func (m model) Init() tea.Cmd { return loadRuns(m.root) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w = max(20, msg.Width)
		m.h = max(chrome+2, msg.Height)
		m.vp.Width = m.w
		m.vp.Height = m.h - chrome
		m.refresh()
		return m, nil

	case runsMsg:
		m.loaded = true
		if msg.err != nil {
			m.err = msg.err.Error()
		}
		m.rows = msg.rows
		m.runCursor = min(m.runCursor, max(0, len(m.rows)-1))
		m.refresh()
		return m, nil

	case resultsMsg:
		if msg.err != nil {
			m.resultErr[msg.runID] = msg.err.Error()
		} else {
			m.results[msg.runID] = msg.rows
		}
		m.refresh()
		return m, nil

	case tea.KeyMsg:
		m.status = ""
		m, cmd := m.key(msg)
		m.refresh()
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	return m.titleLine() + "\n" + m.vp.View() + "\n" + m.statusLine() + "\n" + m.footerLine()
}

// key is the only place a keystroke is interpreted.
func (m model) key(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.help = !m.help
		return m, nil
	case "esc":
		switch m.view {
		case viewRunDetail, viewDiff:
			m.view = viewRuns
		case viewCaseDetail:
			m.view = viewRunDetail
		}
		m.vp.GotoTop()
		return m, nil

	case "up", "k":
		return m.move(-1), nil
	case "down", "j":
		return m.move(1), nil

	case "g":
		switch m.view {
		case viewRuns:
			m.runCursor = 0
		case viewRunDetail:
			m.caseCursor = 0
		default:
			m.vp.GotoTop()
		}
		return m, nil
	case "G":
		switch m.view {
		case viewRuns:
			m.runCursor = max(0, len(m.rows)-1)
		case viewRunDetail:
			m.caseCursor = max(0, len(m.cases())-1)
		default:
			m.vp.GotoBottom()
		}
		return m, nil

	case "pgdown":
		m.vp.ViewDown()
		return m, nil
	case "pgup":
		m.vp.ViewUp()
		return m, nil
	case "ctrl+d":
		m.vp.HalfViewDown()
		return m, nil
	case "ctrl+u":
		m.vp.HalfViewUp()
		return m, nil

	case "enter":
		return m.drillIn()

	case " ":
		if m.view != viewRuns || len(m.rows) == 0 {
			return m, nil
		}
		m.marked = toggleMark(m.marked, m.rows[m.runCursor].man.ID)
		return m, nil
	case "c":
		m.marked = nil
		return m, nil
	case "d":
		return m.startDiff()

	case "n":
		if m.view == viewCaseDetail {
			m.repeat = min(m.repeat+1, max(0, len(m.caseRows())-1))
		}
		return m, nil
	case "p":
		if m.view == viewCaseDetail {
			m.repeat = max(0, m.repeat-1)
		}
		return m, nil
	}
	return m, nil
}

func (m model) move(delta int) model {
	switch m.view {
	case viewRuns:
		if n := len(m.rows); n > 0 {
			m.runCursor = clamp(m.runCursor+delta, 0, n-1)
		}
	case viewRunDetail:
		if n := len(m.cases()); n > 0 {
			m.caseCursor = clamp(m.caseCursor+delta, 0, n-1)
		}
	default:
		if delta > 0 {
			m.vp.LineDown(1)
		} else {
			m.vp.LineUp(1)
		}
	}
	return m
}

func (m model) drillIn() (model, tea.Cmd) {
	switch m.view {
	case viewRuns:
		if len(m.rows) == 0 {
			return m, nil
		}
		row := m.rows[m.runCursor]
		if row.err != "" {
			m.status = row.err
			return m, nil
		}
		m.view = viewRunDetail
		m.caseCursor = 0
		m.vp.GotoTop()
		// Fetch on the way in, not on the way further in, so pressing enter on a
		// case is usually instant.
		if _, ok := m.results[row.man.ID]; !ok {
			if _, bad := m.resultErr[row.man.ID]; !bad {
				return m, loadResults(m.root, row.man.ID)
			}
		}
		return m, nil
	case viewRunDetail:
		if len(m.cases()) == 0 {
			return m, nil
		}
		m.view = viewCaseDetail
		m.repeat = 0
		m.vp.GotoTop()
		return m, nil
	}
	return m, nil
}

// startDiff compares the two marked runs, oldest first.
//
// The marking order deliberately does not decide direction. report.Compare
// treats its first argument as the previous run, and reversed it reports a real
// regression as "no regressions" — the one wrong answer nobody would question.
func (m model) startDiff() (model, tea.Cmd) {
	if m.view != viewRuns {
		return m, nil
	}
	if len(m.marked) < 2 {
		m.status = "mark two runs with space, then press d"
		return m, nil
	}
	a, aok := m.rowByID(m.marked[0])
	b, bok := m.rowByID(m.marked[1])
	if !aok || !bok {
		m.status = "a marked run is no longer in the store"
		return m, nil
	}
	if a.err != "" || b.err != "" {
		m.status = "a marked run has no readable summary"
		return m, nil
	}
	if !olderFirst(a, b) {
		a, b = b, a
	}
	m.diff = report.Compare(a.sum, b.sum)
	m.view = viewDiff
	m.vp.GotoTop()
	return m, nil
}

// olderFirst reports whether a precedes b. The id breaks a timestamp tie so the
// order is total: a --matrix sweep writes runs inside the same second.
func olderFirst(a, b runRow) bool {
	if !a.man.StartedAt.Equal(b.man.StartedAt) {
		return a.man.StartedAt.Before(b.man.StartedAt)
	}
	return a.man.ID < b.man.ID
}

// toggleMark keeps at most two marks. A third press drops the oldest rather than
// being refused, because a keystroke that does nothing reads as a bug.
func toggleMark(marked []string, id string) []string {
	for i, m := range marked {
		if m == id {
			return append(marked[:i:i], marked[i+1:]...)
		}
	}
	marked = append(marked, id)
	if len(marked) > 2 {
		marked = marked[len(marked)-2:]
	}
	return marked
}

func (m model) rowByID(id string) (runRow, bool) {
	for _, r := range m.rows {
		if r.man.ID == id {
			return r, true
		}
	}
	return runRow{}, false
}

// cases is the selected run's case summaries, in the order the suite listed them.
func (m model) cases() []store.CaseSummary {
	if m.runCursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.runCursor].sum.Cases
}

// caseRows is every stored repeat of the selected case.
func (m model) caseRows() []store.ResultRow {
	cs := m.cases()
	if m.caseCursor >= len(cs) || m.runCursor >= len(m.rows) {
		return nil
	}
	id := cs[m.caseCursor].ID
	var out []store.ResultRow
	for _, r := range m.results[m.rows[m.runCursor].man.ID] {
		if r.CaseID == id {
			out = append(out, r)
		}
	}
	return out
}

// refresh keeps View a pure function of state: it is called after every message,
// so no branch can forget to re-render.
func (m *model) refresh() {
	body, cursorLine := m.body()
	m.vp.SetContent(body)
	if cursorLine >= 0 {
		m.ensureVisible(cursorLine)
	}
}

func (m *model) ensureVisible(line int) {
	h := m.vp.Height
	if h < 1 {
		return
	}
	switch {
	case line < m.vp.YOffset:
		m.vp.SetYOffset(line)
	case line >= m.vp.YOffset+h:
		m.vp.SetYOffset(line - h + 1)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
