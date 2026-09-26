package tui

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-runewidth"

	"github.com/mukund1771/evalAgents/internal/eval"
	"github.com/mukund1771/evalAgents/internal/report"
	"github.com/mukund1771/evalAgents/internal/store"
)

// body renders the current view and reports which line the cursor is on, or -1.
//
// The line index is returned by the function that emitted the header rather than
// recomputed by the caller, because a caller guessing "two lines of header" is
// the bug that appears the moment a header gains a line.
func (m model) body() (string, int) {
	switch m.view {
	case viewRunDetail:
		return m.runDetailBody()
	case viewCaseDetail:
		return m.caseDetailBody(), -1
	case viewDiff:
		return m.diffBody(), -1
	default:
		return m.runsBody()
	}
}

func (m model) runsBody() (string, int) {
	if !m.loaded {
		return m.st.dim.Render("loading…"), -1
	}
	if len(m.rows) == 0 {
		// The root goes on its own line and wraps rather than being clipped: the
		// whole point of this message is naming the directory it looked in.
		return "no runs under\n" + renderSpanned(safeLabel(m.root), nil, m.w, m.st) +
			"\n\n" + m.st.dim.Render(clip("run a suite first, or try: agenteval demo", m.w)), -1
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUN\tSUITE\tWHEN\tPASS^1\tCASES")
	for _, r := range m.rows {
		pass, cases := "!", "?"
		if r.err == "" {
			pass = fmt.Sprintf("%.2f", r.sum.Pass1)
			cases = fmt.Sprintf("%d", len(r.sum.Cases))
		}
		// The id and suite come off disk and are printed, so they go through
		// safeLabel like every other untrusted label.
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", safeLabel(r.man.ID), safeLabel(r.man.Suite),
			r.man.StartedAt.Format("2006-01-02 15:04:05"), pass, cases)
	}
	tw.Flush()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	out := make([]string, 0, len(lines))
	// Four columns of gutter on every row, header included: two for the cursor
	// arrow, one for the diff mark, one to keep the mark off the run id.
	out = append(out, "    "+m.st.header.Render(clip(lines[0], m.w-4)))
	for i, line := range lines[1:] {
		// safeLabel guarantees one line per row, so i indexes m.rows. The guard
		// is belt and braces: getting this wrong panicked the whole browser at
		// startup, and an unbrowsable store is worse than a missing row.
		if i >= len(m.rows) {
			break
		}
		// The arrow and the mark both have to be legible with no colour at all.
		arrow, mark := "  ", " "
		if i == m.runCursor {
			arrow = "> "
		}
		if n := strings.TrimSpace(m.markOf(m.rows[i].man.ID)); n != "" {
			mark = m.st.mark.Render(n)
		}
		// Clip rather than let the viewport soft-wrap: a wrapped row would make
		// one run occupy two display rows, and the cursor arithmetic counts rows.
		line = clip(line, m.w-4)
		body := line
		if !m.rows[i].sum.Passed && m.rows[i].err == "" {
			body = m.st.fail.Render(line)
		}
		if i == m.runCursor {
			arrow = m.st.cursor.Render(arrow)
		}
		out = append(out, arrow+mark+" "+body)
	}
	return strings.Join(out, "\n"), 1 + m.runCursor
}

// markOf labels a marked run with the side of the diff it will actually be:
// A is the baseline, B the candidate. It asks diffPair rather than using the
// order the user pressed space in, so the gutter and the report agree.
func (m model) markOf(id string) string {
	base, cur, ok := m.diffPair()
	if !ok {
		for _, marked := range m.marked {
			if marked == id {
				return "*"
			}
		}
		return " "
	}
	switch id {
	case base.man.ID:
		return "A"
	case cur.man.ID:
		return "B"
	}
	return " "
}

// runDetailBody is report.Table's own output with a cursor gutter added.
//
// Reusing the CLI's renderer is what makes "the TUI shows what the CLI shows" a
// structural fact rather than a promise. In particular this view never reads
// CaseSummary.Means itself, so it cannot get the contract wrong that a skipped
// scorer prints "-" and never "0.00".
//
// The gutter goes on after tabwriter has run. Inserting it before would shift the
// cursor row's columns out of line with every other row.
func (m model) runDetailBody() (string, int) {
	if m.runCursor >= len(m.rows) {
		return "", -1
	}
	var buf bytes.Buffer
	report.Table(&buf, m.rows[m.runCursor].sum)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	out := make([]string, 0, len(lines)+2)
	// Line 0 is the run header, line 1 the column header, cases follow.
	out = append(out, "  "+m.st.title.Render(clip(lines[0], m.w-2)))
	if len(lines) > 1 {
		out = append(out, "  "+m.st.header.Render(clip(lines[1], m.w-2)))
	}
	cursorLine := -1
	for i, line := range lines[2:] {
		prefix := "  "
		if i == m.caseCursor {
			prefix = m.st.cursor.Render("> ")
			cursorLine = 2 + i
		}
		// One display row per case, so the gutter and cursorLine stay in step
		// with m.caseCursor however wide the scorer table gets.
		out = append(out, prefix+clip(line, m.w-2))
	}
	if len(m.rows[m.runCursor].sum.Cases) > 0 {
		hint := "  enter a case for its scores, evidence, and trajectory"
		out = append(out, "", m.st.dim.Render(clip(hint, m.w)))
	}
	return strings.Join(out, "\n"), cursorLine
}

func (m model) caseDetailBody() string {
	cases := m.cases()
	if m.caseCursor >= len(cases) {
		return ""
	}
	id := cases[m.caseCursor].ID
	runID := m.rows[m.runCursor].man.ID
	if msg, bad := m.resultErr[runID]; bad {
		return m.st.errText.Render("results.jsonl for this run is unreadable") + "\n" + msg
	}
	rows := m.caseRows()
	if len(rows) == 0 {
		if _, ok := m.results[runID]; !ok {
			return m.st.dim.Render("loading…")
		}
		return fmt.Sprintf("no stored result rows for case %s", id)
	}
	row := rows[min(max(m.repeat, 0), len(rows)-1)]
	width := max(20, m.w-2)

	var b strings.Builder
	head := fmt.Sprintf("case %s", safeLabel(id))
	if len(rows) > 1 {
		// Repeats are the whole reason pass^k exists. Showing only the first row
		// would hide the flake this tool was built to find.
		head += fmt.Sprintf("   repeat %d/%d  (n/p)", m.repeat+1, len(rows))
	}
	head += fmt.Sprintf("   %.1fms", row.DurationMS)
	b.WriteString(m.st.title.Render(head) + "\n")

	if row.Error != "" {
		// A harness failure is not a failed case, and the distinction is worth
		// keeping visible.
		b.WriteString(m.st.errText.Render("harness error: ") +
			renderSpanned(row.Error, nil, width, m.st) + "\n")
	}
	if mt := row.Trajectory.Metrics; mt.Tokens > 0 || mt.Cost > 0 {
		line := fmt.Sprintf("target reported: %d tokens", mt.Tokens)
		if mt.Cost > 0 {
			line += fmt.Sprintf(", cost %.4f", mt.Cost)
		}
		b.WriteString(m.st.dim.Render(line) + "\n")
	}

	b.WriteString("\n" + m.st.header.Render("FINAL OUTPUT") + "\n")
	b.WriteString(renderSpanned(row.Trajectory.FinalOutput, nil, width, m.st) + "\n")

	b.WriteString("\n" + m.st.header.Render("SCORES") + "\n")
	for _, sc := range row.Scores {
		b.WriteString(m.scoreLine(sc, row.Trajectory, width) + "\n")
	}

	b.WriteString("\n" + m.st.header.Render("TRAJECTORY") + "\n")
	b.WriteString(m.steps(row, width))
	return b.String()
}

// scoreLine renders one score, keeping nil distinct from zero and naming any
// evidence span the scorer recorded that does not fit the step it points at.
func (m model) scoreLine(sc eval.Score, tr eval.Trajectory, width int) string {
	val, style := "-", m.st.dim
	if sc.Value != nil {
		val = fmt.Sprintf("%.2f", *sc.Value)
		style = m.st.fail
		if sc.OK() {
			style = m.st.pass
		}
	}
	// Name and value on one line, reason beneath it. Keeping the reason on the
	// same line means a long explanation wraps back to column zero, which breaks
	// the alignment that makes a column of scores scannable.
	line := fmt.Sprintf("  %-28s %s", safeLabel(sc.Name), style.Render(val))
	if sc.Explanation != "" {
		line += "\n      " + indent(renderSpanned(sc.Explanation, nil, width-6, m.st))
	}
	for _, sp := range sc.Evidence {
		content := ""
		if sp.StepIndex >= 0 && sp.StepIndex < len(tr.Steps) {
			content = tr.Steps[sp.StepIndex].Content
		}
		_, bad := spanIntervals(content, []eval.Span{sp}, sp.StepIndex)
		suffix := ""
		if len(bad) > 0 {
			// Print the numbers exactly as stored. A span that does not fit its
			// step means the scorer that produced it is wrong, and hiding that
			// would leave the user wondering why nothing is highlighted.
			suffix = m.st.errText.Render("  out of range")
		}
		line += fmt.Sprintf("\n      evidence step %d [%d,%d)%s", sp.StepIndex, sp.Start, sp.End, suffix)
	}
	return line
}

// steps renders the trajectory with every recorded evidence span marked in the
// step it names.
func (m model) steps(row store.ResultRow, width int) string {
	byStep := map[int][]eval.Span{}
	for _, sc := range row.Scores {
		for _, sp := range sc.Evidence {
			byStep[sp.StepIndex] = append(byStep[sp.StepIndex], sp)
		}
	}
	var b strings.Builder
	for i, step := range row.Trajectory.Steps {
		// Kind and CallID are decoded from the evaluated program's stdout, so
		// they are exactly as untrusted as Content and go through safeLabel too.
		label := fmt.Sprintf("  [%d] %s", i, safeLabel(step.Kind))
		if step.CallID != "" {
			label += " call=" + safeLabel(step.CallID)
		}
		if step.IsError {
			label += m.st.fail.Render(" error")
		}
		b.WriteString(m.st.dim.Render(label) + "\n")
		for _, tc := range step.ToolCalls {
			args := strings.TrimSpace(string(tc.Args))
			if args == "" {
				args = "{}"
			}
			b.WriteString("      " + indent(renderSpanned(tc.Name+" "+args, nil, width-6, m.st)) + "\n")
		}
		if step.Content != "" {
			ivs, _ := spanIntervals(step.Content, byStep[i], i)
			b.WriteString("      " + indent(renderSpanned(step.Content, ivs, width-6, m.st)) + "\n")
		}
	}
	return b.String()
}

// indent keeps wrapped continuation lines under the same margin as the first.
func indent(s string) string {
	return strings.ReplaceAll(s, "\n", "\n      ")
}

// diffBody is report.FormatDiff's output, coloured by line prefix and nothing
// else. The words "regression:" and "note:" are already there, which is the
// channel that survives a terminal with no colour.
func (m model) diffBody() string {
	lines := strings.Split(strings.TrimRight(report.FormatDiff(m.diff), "\n"), "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "regression:"):
			lines[i] = "  " + m.st.regress.Render(line)
		case strings.HasPrefix(line, "note:"):
			lines[i] = "  " + m.st.note.Render(line)
		default:
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) titleLine() string {
	name := map[view]string{
		viewRuns:       "runs",
		viewRunDetail:  "run",
		viewCaseDetail: "case",
		viewDiff:       "diff",
	}[m.view]
	head := "agenteval " + name
	// Clamp to the terminal: the three chrome lines are outside the viewport, so
	// nothing else truncates them, and one overlong line makes the alt screen
	// scroll and pushes the title off the top.
	root := clip(safeLabel(m.root), max(0, m.w-runewidth.StringWidth(head)-3))
	return m.st.title.Render(head) + m.st.dim.Render("   "+root)
}

func (m model) statusLine() string {
	switch {
	// status before err: err is set once by a failed load and never cleared, so
	// giving it precedence made every later keypress look like it did nothing.
	case m.status != "":
		return m.st.note.Render(clip(m.status, m.w))
	case m.err != "":
		return m.st.errText.Render(clip(safeLabel(m.err), m.w))
	case m.view == viewRuns && len(m.marked) > 0:
		return m.st.dim.Render(clip(fmt.Sprintf("marked %s", strings.Join(m.marked, "  ")), m.w))
	}
	return ""
}

func (m model) footerLine() string {
	if !m.help {
		return m.st.dim.Render("?  keys")
	}
	// Kept inside 80 cells so the default terminal does not wrap the footer and
	// push the whole fixed-height view one row too tall.
	keys := map[view]string{
		viewRuns:       "jk move · enter open · space mark · d diff · c clear · g/G ends · q quit",
		viewRunDetail:  "jk case · enter open · esc back · g/G ends · q quit",
		viewCaseDetail: "jk scroll · n/p repeat · pgup/pgdn · ctrl+u/d · esc back · q quit",
		viewDiff:       "jk scroll · pgup/pgdn · esc back · q quit",
	}[m.view]
	return m.st.dim.Render(clip(keys, m.w))
}
