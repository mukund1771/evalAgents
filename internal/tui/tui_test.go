package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mukund1771/evalAgents/internal/eval"
	"github.com/mukund1771/evalAgents/internal/report"
	"github.com/mukund1771/evalAgents/internal/store"
)

// Fixtures are written through the store API rather than by running a suite, so
// these tests do not depend on YAML, cassettes, or the runner.
func writeRun(t *testing.T, root, id string, when time.Time, suiteName string, scorers []string, rows []store.ResultRow) store.Summary {
	t.Helper()
	run, err := store.Create(root, id)
	if err != nil {
		t.Fatal(err)
	}
	man := store.Manifest{
		ID:        id,
		Suite:     suiteName,
		StartedAt: when.UTC(),
		Target:    "replay",
		Scorers:   scorers,
		Repeats:   1,
	}
	var order []string
	seen := map[string]bool{}
	for _, r := range rows {
		if !seen[r.CaseID] {
			seen[r.CaseID] = true
			order = append(order, r.CaseID)
		}
	}
	sum := store.Summarize(man, rows, order)
	if err := run.WriteManifest(man); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteResults(rows); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteSummary(sum); err != nil {
		t.Fatal(err)
	}
	return sum
}

// row builds one result row whose trajectory has a user step and a final step,
// so evidence spans have somewhere real to point.
func row(caseID, final string, scores ...eval.Score) store.ResultRow {
	return store.ResultRow{
		CaseID: caseID,
		Trajectory: eval.Trajectory{
			CaseID: caseID,
			Steps: []eval.Step{
				{Kind: "user", Content: "where is my order"},
				{Kind: "final", Content: final},
			},
			FinalOutput: final,
			Metrics:     eval.Metrics{Tokens: 12},
		},
		Scores:     scores,
		DurationMS: 1.5,
	}
}

// newTestModel loads the store synchronously, so there is no program, no
// goroutine and no terminal anywhere in these tests.
func newTestModel(t *testing.T, root string) model {
	t.Helper()
	m := newModel(root, plainStyles())
	msg := loadRuns(root)()
	out, _ := m.Update(msg)
	m = out.(model)
	out, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return out.(model)
}

// loadResultsFor feeds in the results for a run the way the real command would.
func loadResultsFor(t *testing.T, m model, id string) model {
	t.Helper()
	out, _ := m.Update(loadResults(m.root, id)())
	return out.(model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func drive(t *testing.T, m model, keys ...string) (model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		out, c := m.Update(key(k))
		next, ok := out.(model)
		if !ok {
			t.Fatalf("Update returned %T, not a model", out)
		}
		m, cmd = next, c
	}
	return m, cmd
}

// twoRuns writes a green run and, one second later, a run where case beta broke.
func twoRuns(t *testing.T, root string) (goodID, badID string) {
	t.Helper()
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	scorers := []string{"contains", "json_valid"}

	pass := func(id string) store.ResultRow {
		return row(id, "shipped",
			eval.Float("contains", 1, 1, "final output contains the expected text",
				[]eval.Span{{StepIndex: 1, Start: 0, End: 7}}),
			eval.Skip("json_valid", "skipped: expected.json is not true"),
		)
	}
	writeRun(t, root, "20260926T100000Z-aaaaaa", base, "support", scorers,
		[]store.ResultRow{pass("alpha"), pass("beta")})

	writeRun(t, root, "20260926T100001Z-bbbbbb", base.Add(time.Second), "support", scorers,
		[]store.ResultRow{
			pass("alpha"),
			row("beta", "lost",
				eval.Float("contains", 0, 1, `final output does not contain "shipped"`,
					[]eval.Span{{StepIndex: 1, Start: 0, End: 4}}),
				eval.Skip("json_valid", "skipped: expected.json is not true"),
			),
		})
	return "20260926T100000Z-aaaaaa", "20260926T100001Z-bbbbbb"
}

func TestRunsListIsNewestFirst(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	good, bad := twoRuns(t, root)
	m := newTestModel(t, root)

	body, cursor := m.body()
	if cursor != 1 {
		t.Fatalf("cursor line = %d, want the first row", cursor)
	}
	iGood, iBad := strings.Index(body, good), strings.Index(body, bad)
	if iGood < 0 || iBad < 0 {
		t.Fatalf("both runs should be listed:\n%s", body)
	}
	if iBad > iGood {
		t.Fatalf("newest run should come first:\n%s", body)
	}
	if !strings.Contains(body, "PASS^1") {
		t.Fatalf("runs list should show pass^1:\n%s", body)
	}
}

func TestEmptyStoreSaysSoInsteadOfGoingBlank(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	m := newTestModel(t, root)
	body, _ := m.body()
	if !strings.Contains(body, "no runs") || !strings.Contains(body, root) {
		t.Fatalf("empty state should name the root:\n%s", body)
	}
	if v := m.View(); v == "" {
		t.Fatal("View is empty on an empty store")
	}
}

func TestNavigationDrillsInAndEscapesBack(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	_, bad := twoRuns(t, root)
	m := newTestModel(t, root)

	// The newest run is selected, so enter opens the regressed one.
	m, _ = drive(t, m, "enter")
	if m.view != viewRunDetail {
		t.Fatalf("view = %v, want run detail", m.view)
	}
	body, _ := m.body()
	if !strings.Contains(body, bad) {
		t.Fatalf("run detail should name the selected run:\n%s", body)
	}

	m = loadResultsFor(t, m, bad)
	m, _ = drive(t, m, "j", "enter")
	if m.view != viewCaseDetail {
		t.Fatalf("view = %v, want case detail", m.view)
	}
	if body, _ := m.body(); !strings.Contains(body, "case beta") {
		t.Fatalf("case detail should be on beta:\n%s", body)
	}

	m, _ = drive(t, m, "esc")
	if m.view != viewRunDetail {
		t.Fatalf("esc from a case went to %v", m.view)
	}
	m, _ = drive(t, m, "esc")
	if m.view != viewRuns || m.runCursor != 0 {
		t.Fatalf("esc to runs: view=%v cursor=%d", m.view, m.runCursor)
	}

	// Cursor movement must not run off either end.
	m, _ = drive(t, m, "k", "k", "k")
	if m.runCursor != 0 {
		t.Fatalf("cursor went above the first row: %d", m.runCursor)
	}
	m, _ = drive(t, m, "j", "j", "j", "j")
	if m.runCursor != len(m.rows)-1 {
		t.Fatalf("cursor = %d, want %d", m.runCursor, len(m.rows)-1)
	}
}

// The run detail view must be report.Table's own output, because that is what
// guarantees it cannot disagree with the CLI about a number.
func TestRunDetailIsTheCLITableAndKeepsSkippedAsDash(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	twoRuns(t, root)
	m := newTestModel(t, root)
	m, _ = drive(t, m, "enter")

	sum := m.rows[m.runCursor].sum
	var want bytes.Buffer
	report.Table(&want, sum)
	body, _ := m.body()

	at := 0
	for _, line := range strings.Split(strings.TrimRight(want.String(), "\n"), "\n") {
		i := strings.Index(body[at:], line)
		if i < 0 {
			t.Fatalf("run detail is missing the CLI table line %q:\n%s", line, body)
		}
		at += i + len(line)
	}

	// json_valid skipped on every case, so its column must be "-" and never 0.00.
	col := -1
	for i, sc := range sum.Scorers {
		if sc.Name == "json_valid" {
			col = i
		}
	}
	if col < 0 {
		t.Fatal("fixture lost its skipped scorer")
	}
	for _, line := range strings.Split(body, "\n") {
		// Strip the two-column cursor gutter before reading the row.
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == ">" {
			f = f[1:]
		}
		if len(f) == 0 || f[0] != "alpha" {
			continue
		}
		// id, pass^1, pass^k, then one cell per scorer.
		if len(f) < 4+col {
			t.Fatalf("row has %d fields: %q", len(f), line)
		}
		if got := f[3+col]; got != "-" {
			t.Fatalf("skipped scorer rendered as %q, want \"-\": %q", got, line)
		}
		return
	}
	t.Fatalf("no row for case alpha:\n%s", body)
}

func TestCaseDetailShowsScoresEvidenceAndMetrics(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	_, bad := twoRuns(t, root)
	m := newTestModel(t, root)
	m, _ = drive(t, m, "enter")
	m = loadResultsFor(t, m, bad)
	m, _ = drive(t, m, "enter")

	body, _ := m.body()
	for _, want := range []string{
		"case alpha",
		"1.00",                     // the applied score
		"-",                        // the skipped one
		"skipped: expected.json",   // and its reason
		"evidence step 1 [0,7)",    // the span, printed as stored
		"target reported: 12 toke", // parity with agenteval show
		"FINAL OUTPUT",
		"TRAJECTORY",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("case detail missing %q:\n%s", want, body)
		}
	}

	// The span names bytes 0..7 of the final step, which is "shipped".
	if !strings.Contains(body, evidenceOpen+"shipped"+evidenceClose) {
		t.Fatalf("evidence was not marked in the trajectory:\n%s", body)
	}
	// And it must mark only those bytes.
	if strings.Contains(body, evidenceOpen+"where is my order") {
		t.Fatalf("evidence marked the wrong step:\n%s", body)
	}
}

// A span the scorer cannot stand behind is reported with its raw numbers rather
// than quietly producing no highlight.
func TestCaseDetailReportsAnOutOfRangeSpan(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	writeRun(t, root, "20260926T100000Z-cccccc", time.Now(), "judged", []string{"model_graded"},
		[]store.ResultRow{row("alpha", "short",
			eval.Float("model_graded", 0.9, 0.5, "the judge liked it",
				[]eval.Span{{StepIndex: 1, Start: 0, End: 9999}}),
		)})
	m := newTestModel(t, root)
	m, _ = drive(t, m, "enter")
	m = loadResultsFor(t, m, "20260926T100000Z-cccccc")
	m, _ = drive(t, m, "enter")

	body, _ := m.body()
	if !strings.Contains(body, "evidence step 1 [0,9999)") {
		t.Fatalf("the raw span should still be printed:\n%s", body)
	}
	if !strings.Contains(body, "out of range") {
		t.Fatalf("an impossible span should be called out:\n%s", body)
	}
	if strings.Contains(body, evidenceOpen) {
		t.Fatalf("nothing should be highlighted from a refused span:\n%s", body)
	}
}

func TestCaseDetailStepsThroughRepeats(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	id := "20260926T100000Z-dddddd"
	first := row("alpha", "shipped", eval.Float("contains", 1, 1, "matched", nil))
	second := row("alpha", "lost", eval.Float("contains", 0, 1, "flaked on the second try", nil))
	second.Repeat = 1
	writeRun(t, root, id, time.Now(), "flaky", []string{"contains"},
		[]store.ResultRow{first, second})

	m := newTestModel(t, root)
	m, _ = drive(t, m, "enter")
	m = loadResultsFor(t, m, id)
	m, _ = drive(t, m, "enter")

	if body, _ := m.body(); !strings.Contains(body, "repeat 1/2") || !strings.Contains(body, "matched") {
		t.Fatalf("first repeat:\n%s", body)
	}
	m, _ = drive(t, m, "n")
	if body, _ := m.body(); !strings.Contains(body, "repeat 2/2") || !strings.Contains(body, "flaked") {
		t.Fatalf("second repeat:\n%s", body)
	}
	// n past the end clamps rather than wrapping or panicking.
	m, _ = drive(t, m, "n", "n")
	if m.repeat != 1 {
		t.Fatalf("repeat = %d, want it clamped to 1", m.repeat)
	}
	m, _ = drive(t, m, "p", "p", "p")
	if m.repeat != 0 {
		t.Fatalf("repeat = %d, want 0", m.repeat)
	}
}

// Marking order must not be able to reverse the comparison: Compare(new, old)
// reports a real regression as "no regressions".
func TestDiffAlwaysUsesTheOlderRunAsBaseline(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	good, bad := twoRuns(t, root)
	m := newTestModel(t, root)

	// Cursor starts on the newest run, so this marks the newer one first.
	m, _ = drive(t, m, " ", "j", " ", "d")
	if m.view != viewDiff {
		t.Fatalf("view = %v, want diff", m.view)
	}
	if m.diff.Baseline != good || m.diff.Current != bad {
		t.Fatalf("diff is %s -> %s, want %s -> %s", m.diff.Baseline, m.diff.Current, good, bad)
	}
	body := m.diffBody()
	if !strings.Contains(body, "regression: case beta passed and now fails") {
		t.Fatalf("diff did not report the regression:\n%s", body)
	}
	if strings.Contains(body, "no regressions") {
		t.Fatalf("diff ran backwards:\n%s", body)
	}
	m, _ = drive(t, m, "esc")
	if m.view != viewRuns {
		t.Fatalf("esc from diff went to %v", m.view)
	}
}

// The gutter said A for whichever run was marked first while the diff used the
// older run as baseline, so the list claimed A -> B and the report did the
// opposite. Both now read from diffPair, and this is what keeps them together.
func TestMarkLabelsMatchTheDiffDirection(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	good, bad := twoRuns(t, root)
	m := newTestModel(t, root)

	// Cursor starts on the newest run, so the newer one is marked first.
	m, _ = drive(t, m, " ", "j", " ")
	if got := m.markOf(good); got != "A" {
		t.Fatalf("older run %s labelled %q, want A (the baseline)", good, got)
	}
	if got := m.markOf(bad); got != "B" {
		t.Fatalf("newer run %s labelled %q, want B (the candidate)", bad, got)
	}

	// And the label has to name the run the comparison really uses.
	m, _ = drive(t, m, "d")
	if m.diff.Baseline != good || m.diff.Current != bad {
		t.Fatalf("diff %s -> %s but gutter said A=%s B=%s",
			m.diff.Baseline, m.diff.Current, good, bad)
	}

	// The mark must not collide with the run id in the gutter.
	body, _ := m.runsBody()
	for _, line := range strings.Split(body, "\n") {
		if i := strings.Index(line, good); i > 0 && line[i-1] != ' ' {
			t.Fatalf("mark runs into the run id: %q", line)
		}
	}
}

func TestDiffNeedsTwoMarksAndSaysSo(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	twoRuns(t, root)
	m := newTestModel(t, root)

	m, _ = drive(t, m, "d")
	if m.view != viewRuns || !strings.Contains(m.status, "space") {
		t.Fatalf("d with no marks: view=%v status=%q", m.view, m.status)
	}
	m, _ = drive(t, m, " ", "d")
	if m.view != viewRuns || m.status == "" {
		t.Fatalf("d with one mark: view=%v status=%q", m.view, m.status)
	}
}

func TestSpaceKeepsAtMostTwoMarks(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"r-aaa", "r-bbb", "r-ccc"} {
		writeRun(t, root, id, base.Add(time.Duration(i)*time.Second), "s", []string{"contains"},
			[]store.ResultRow{row("alpha", "ok", eval.Float("contains", 1, 1, "m", nil))})
	}
	m := newTestModel(t, root)
	m, _ = drive(t, m, " ", "j", " ", "j", " ")
	if len(m.marked) != 2 {
		t.Fatalf("marked = %#v, want two", m.marked)
	}
	// Re-marking the same run clears it.
	before := len(m.marked)
	m, _ = drive(t, m, " ")
	if len(m.marked) != before-1 {
		t.Fatalf("space did not unmark: %#v", m.marked)
	}
	m, _ = drive(t, m, "c")
	if len(m.marked) != 0 {
		t.Fatalf("c did not clear marks: %#v", m.marked)
	}
}

func TestQuitAndHelp(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	twoRuns(t, root)
	m := newTestModel(t, root)

	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := drive(t, m, k)
		if cmd == nil {
			t.Fatalf("%s returned no command", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s did not quit", k)
		}
	}

	height := m.vp.Height
	m, _ = drive(t, m, "?")
	if !m.help || !strings.Contains(m.footerLine(), "quit") {
		t.Fatalf("help did not open: %q", m.footerLine())
	}
	if m.vp.Height != height {
		t.Fatalf("help resized the viewport %d -> %d", height, m.vp.Height)
	}
	m, _ = drive(t, m, "?")
	if m.help {
		t.Fatal("help did not close")
	}
}

// The view is a fixed number of lines so the alt screen never scrolls.
func TestViewFillsTheTerminalExactly(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	id := "20260926T100000Z-eeeeee"
	long := strings.Repeat("a long line of agent output\n", 500)
	writeRun(t, root, id, time.Now(), "wordy", []string{"contains"},
		[]store.ResultRow{row("alpha", long, eval.Float("contains", 1, 1, "m", nil))})

	m := newTestModel(t, root)
	if got, want := strings.Count(m.View(), "\n"), m.h-1; got != want {
		t.Fatalf("View has %d newlines, want %d for a %d-line terminal", got, want, m.h)
	}

	m, _ = drive(t, m, "enter")
	m = loadResultsFor(t, m, id)
	m, _ = drive(t, m, "enter")
	if m.vp.YOffset != 0 {
		t.Fatalf("case detail opened scrolled to %d", m.vp.YOffset)
	}
	m, _ = drive(t, m, "pgdown")
	if m.vp.YOffset == 0 {
		t.Fatal("pgdown did not scroll")
	}
	m, _ = drive(t, m, "G")
	if !m.vp.AtBottom() {
		t.Fatal("G did not reach the bottom")
	}
	if got, want := strings.Count(m.View(), "\n"), m.h-1; got != want {
		t.Fatalf("scrolled View has %d newlines, want %d", got, want)
	}
}

// Nothing on disk is trusted to be well formed, and a browser that panics on a
// corrupt file is worse than one that says the file is corrupt.
func TestCorruptStoreRendersInsteadOfPanicking(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agenteval")
	good, _ := twoRuns(t, root)

	// A run whose summary will not parse. The manifest stays valid so the row is
	// still listed.
	broken := "20260926T100002Z-ffffff"
	writeRun(t, root, broken, time.Now(), "broken", []string{"contains"},
		[]store.ResultRow{row("alpha", "ok", eval.Float("contains", 1, 1, "m", nil))})
	if err := os.WriteFile(filepath.Join(root, "runs", broken, "summary.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	// And a run whose results will not parse.
	if err := os.WriteFile(filepath.Join(root, "runs", good, "results.jsonl"), []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, root)
	body, _ := m.body()
	if !strings.Contains(body, broken) {
		t.Fatalf("a run with a corrupt summary should still be listed:\n%s", body)
	}
	if !strings.Contains(body, "!") {
		t.Fatalf("a corrupt run should be flagged:\n%s", body)
	}
	// Entering it reports the problem rather than opening a blank pane.
	for i := range m.rows {
		if m.rows[i].man.ID != broken {
			continue
		}
		m.runCursor = i
	}
	m, _ = drive(t, m, "enter")
	if m.view != viewRuns || m.status == "" {
		t.Fatalf("entering a corrupt run: view=%v status=%q", m.view, m.status)
	}

	// The unreadable results file surfaces in the case view.
	for i := range m.rows {
		if m.rows[i].man.ID != good {
			continue
		}
		m.runCursor = i
	}
	m, _ = drive(t, m, "enter")
	m = loadResultsFor(t, m, good)
	m, _ = drive(t, m, "enter")
	if body := m.caseDetailBody(); !strings.Contains(body, "unreadable") {
		t.Fatalf("a corrupt results file should be reported:\n%s", body)
	}
}

func TestRootThatIsAFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, path)
	if m.err == "" {
		t.Fatal("a root that is a file should set an error")
	}
	if !strings.Contains(m.View(), m.err) {
		t.Fatalf("the error should be visible:\n%s", m.View())
	}
}

func TestRenderBeforeAnyDataDoesNotPanic(t *testing.T) {
	m := newModel(filepath.Join(t.TempDir(), ".agenteval"), plainStyles())
	if v := m.View(); v == "" {
		t.Fatal("View before loading is empty")
	}
	if body, _ := m.body(); !strings.Contains(body, "loading") {
		t.Fatalf("body before loading = %q", body)
	}
	// A size message arriving before the data must be harmless too.
	out, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if v := out.(model).View(); v == "" {
		t.Fatal("View after resize but before loading is empty")
	}
	// And every key pressed on an empty store must be harmless.
	drive(t, out.(model), "enter", "j", "k", " ", "d", "c", "n", "p", "G", "g", "esc", "?")
}
