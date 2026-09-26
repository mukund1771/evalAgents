package scorer

import (
	"encoding/json"
	"testing"

	"github.com/mukund1771/evalAgents/internal/eval"
)

func caseWith(exp any) eval.Case {
	b, err := json.Marshal(exp)
	if err != nil {
		panic(err)
	}
	return eval.Case{ID: "c", Expected: b}
}

func traj(final string, steps ...eval.Step) eval.Trajectory {
	return eval.Trajectory{CaseID: "c", Steps: steps, FinalOutput: final}
}

func val(t *testing.T, s eval.Score) float64 {
	t.Helper()
	if s.Value == nil {
		t.Fatalf("score %s skipped: %s", s.Name, s.Explanation)
	}
	return *s.Value
}

func TestOutputScorers(t *testing.T) {
	final := eval.Step{Kind: "final", Content: `{"ok":true}`}
	tr := traj(`{"ok":true}`, eval.Step{Kind: "user", Content: "hi"}, final)

	if got := val(t, ExactMatch("exact_match", 1).Score(caseWith(map[string]any{"exact": `{"ok":true}`}), tr)); got != 1 {
		t.Fatalf("exact = %v", got)
	}
	miss := ExactMatch("exact_match", 1).Score(caseWith(map[string]any{"exact": "nope"}), tr)
	if val(t, miss) != 0 || len(miss.Evidence) != 1 || miss.Evidence[0].StepIndex != 1 {
		t.Fatalf("exact miss = %#v", miss)
	}
	if ExactMatch("exact_match", 1).Score(caseWith(map[string]any{}), tr).Value != nil {
		t.Fatal("exact should skip without expected.exact")
	}

	if got := val(t, Contains("contains", 1).Score(caseWith(map[string]any{"contains": "ok"}), tr)); got != 1 {
		t.Fatalf("contains = %v", got)
	}
	if Contains("contains", 1).Score(caseWith(map[string]any{}), tr).Value != nil {
		t.Fatal("contains should skip")
	}

	if got := val(t, JSONValid("json_valid", 1).Score(caseWith(map[string]any{"json": true}), tr)); got != 1 {
		t.Fatalf("json = %v", got)
	}
	bad := traj("not-json", eval.Step{Kind: "final", Content: "not-json"})
	if got := val(t, JSONValid("json_valid", 1).Score(caseWith(map[string]any{"json": true}), bad)); got != 0 {
		t.Fatalf("invalid json scored %v", got)
	}
	if JSONValid("json_valid", 1).Score(caseWith(map[string]any{}), tr).Value != nil {
		t.Fatal("json_valid should skip unless expected.json")
	}
}

// A cmd target sets FinalOutput and the final step's Content independently, so
// an offset found in one is not valid in the other. The span is documented as
// indexing the step, and a TUI that paints it would otherwise paint the wrong
// bytes or slice out of range.
func TestContainsEvidenceIndexesTheStepNotFinalOutput(t *testing.T) {
	// "ok" is at offset 7 of FinalOutput and offset 0 of the step's content.
	tr := traj("prefix ok", eval.Step{Kind: "final", Content: "ok tail"})
	sc := Contains("contains", 1).Score(caseWith(map[string]any{"contains": "ok"}), tr)
	if val(t, sc) != 1 {
		t.Fatalf("score = %#v", sc)
	}
	if len(sc.Evidence) != 1 {
		t.Fatalf("evidence = %#v", sc.Evidence)
	}
	ev := sc.Evidence[0]
	if ev.StepIndex != 0 || ev.Start != 0 || ev.End != 2 {
		t.Fatalf("evidence = %#v, want step 0 [0,2) into %q", ev, tr.Steps[0].Content)
	}
	// The span must be slicable against the step it names.
	got := tr.Steps[ev.StepIndex].Content[ev.Start:ev.End]
	if got != "ok" {
		t.Fatalf("span selects %q, want %q", got, "ok")
	}
}

// When the matched text lives only in FinalOutput, there are no honest offsets
// to report, so the span covers the whole step instead of inventing a range.
func TestContainsEvidenceFallsBackToTheWholeStep(t *testing.T) {
	tr := traj("the answer is 42", eval.Step{Kind: "final", Content: "see above"})
	sc := Contains("contains", 1).Score(caseWith(map[string]any{"contains": "42"}), tr)
	if val(t, sc) != 1 {
		t.Fatalf("score = %#v", sc)
	}
	ev := sc.Evidence[0]
	if ev.StepIndex != 0 || ev.Start != 0 || ev.End != len("see above") {
		t.Fatalf("evidence = %#v, want the whole step", ev)
	}
}
