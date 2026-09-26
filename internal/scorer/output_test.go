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
