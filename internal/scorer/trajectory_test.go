package scorer

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/mukund1771/evalAgents/internal/eval"
)

func toolStep(name, args string) eval.Step {
	return eval.Step{Kind: "assistant", ToolCalls: []eval.ToolCall{{Name: name, Args: json.RawMessage(args)}}}
}

func TestTrajectoryScorers(t *testing.T) {
	exp := map[string]any{
		"tools": []map[string]any{
			{"name": "lookup_order", "args": map[string]string{"order_id": "1001"}},
			{"name": "issue_refund", "args": map[string]string{"order_id": "1001", "reason": "damaged"}},
		},
	}
	good := traj("refund issued",
		eval.Step{Kind: "user", Content: "refund"},
		toolStep("lookup_order", `{"order_id":"1001"}`),
		eval.Step{Kind: "tool_result", CallID: "lookup_order", Content: "{}"},
		toolStep("issue_refund", `{"reason":"damaged","order_id":"1001"}`),
		eval.Step{Kind: "tool_result", CallID: "issue_refund", Content: "{}"},
		eval.Step{Kind: "final", Content: "refund issued"},
	)
	c := caseWith(exp)

	if got := val(t, ToolTrajectoryExact("tool_trajectory_exact", 1).Score(c, good)); got != 1 {
		t.Fatalf("exact trajectory %v", got)
	}
	if got := val(t, ToolTrajectorySubsequence("tool_trajectory_subsequence", 1).Score(c, good)); got != 1 {
		t.Fatalf("subsequence %v", got)
	}
	if got := val(t, ToolCallF1("tool_call_accuracy", 1).Score(c, good)); got != 1 {
		t.Fatalf("f1 %v", got)
	}
	if got := val(t, ToolsSucceeded("tools_succeeded", 1).Score(c, good)); got != 1 {
		t.Fatalf("tools ok %v", got)
	}
	if got := val(t, StepsWithin("steps_within", 6, 1).Score(c, good)); got != 1 {
		t.Fatalf("steps %v", got)
	}
	if got := val(t, NoLoop("no_loop", 2, 1).Score(c, good)); got != 1 {
		t.Fatalf("loop %v", got)
	}

	// Partial credit: only the first tool, so subsequence is 0.5 and F1 is 2*(1/1)*(1/2)/(1+0.5) = 2/3.
	partial := traj("nope",
		toolStep("lookup_order", `{"order_id":"1001"}`),
		eval.Step{Kind: "final", Content: "nope"},
	)
	sub := ToolTrajectorySubsequence("tool_trajectory_subsequence", 1).Score(c, partial)
	if math.Abs(val(t, sub)-0.5) > 1e-9 {
		t.Fatalf("subsequence partial = %v", val(t, sub))
	}
	f1 := ToolCallF1("tool_call_accuracy", 1).Score(c, partial)
	if math.Abs(val(t, f1)-2.0/3.0) > 1e-9 {
		t.Fatalf("f1 partial = %v", val(t, f1))
	}
	if ToolTrajectoryExact("tool_trajectory_exact", 1).Score(c, partial).OK() {
		t.Fatal("exact trajectory should fail")
	}

	looped := traj("x",
		toolStep("lookup_order", `{"order_id":"1001"}`),
		toolStep("lookup_order", `{"order_id":"1001"}`),
		toolStep("lookup_order", `{"order_id":"1001"}`),
	)
	loop := NoLoop("no_loop", 2, 1).Score(c, looped)
	if loop.OK() || len(loop.Evidence) != 1 || loop.Evidence[0].StepIndex != 2 {
		t.Fatalf("loop score = %#v", loop)
	}

	errored := traj("x", eval.Step{Kind: "tool_result", CallID: "lookup_order", IsError: true, Content: "boom"})
	errScore := ToolsSucceeded("tools_succeeded", 1).Score(c, errored)
	if errScore.OK() || len(errScore.Evidence) != 1 {
		t.Fatalf("tool error = %#v", errScore)
	}

	over := StepsWithin("steps_within", 1, 1).Score(c, good)
	if over.OK() {
		t.Fatal("step budget should fail")
	}

	if ToolTrajectoryExact("tool_trajectory_exact", 1).Score(caseWith(map[string]any{}), good).Value != nil {
		t.Fatal("tool scorers skip without expected.tools")
	}
}
