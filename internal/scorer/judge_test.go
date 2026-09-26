package scorer

import (
	"context"
	"testing"

	"github.com/mukund1771/evalAgents/internal/eval"
	"github.com/mukund1771/evalAgents/internal/suite"
)

type scripted struct {
	replies []string
	calls   int
}

func (s *scripted) Complete(ctx context.Context, system, user string) (string, error) {
	if s.calls >= len(s.replies) {
		return "", context.Canceled
	}
	r := s.replies[s.calls]
	s.calls++
	return r, nil
}

func TestBuildAlias(t *testing.T) {
	got, err := Build([]suite.ScorerSpec{{Kind: "tool_call_accuracy", As: "tool_call_accuracy"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name() != "tool_call_accuracy" {
		t.Fatalf("name %s", got[0].Name())
	}
}

func TestJudgeSchemaRetry(t *testing.T) {
	tr := traj("refund issued", eval.Step{Kind: "final", Content: "refund issued"})
	c := eval.Case{ID: "c", Input: []byte(`{"user":"refund"}`)}

	badThenGood := &scripted{replies: []string{
		"not json",
		`{"value":1,"passed":true,"explanation":"done","evidence":[{"step_index":0,"start":0,"end":4}]}`,
	}}
	s := ModelGraded("task_completion", "task_completion", 1, badThenGood, true).Score(c, tr)
	if s.Value == nil || *s.Value != 1 || !s.OK() {
		t.Fatalf("retry should accept the second reply: %#v", s)
	}
	if badThenGood.calls != 2 {
		t.Fatalf("calls = %d", badThenGood.calls)
	}

	inert := &scripted{replies: []string{"nope", "still nope"}}
	skipped := ModelGraded("task_completion", "task_completion", 1, inert, true).Score(c, tr)
	if skipped.Value != nil {
		t.Fatalf("invalid judge output must be skipped, got %#v", skipped)
	}

	off := ModelGraded("task_completion", "faithfulness", 1, inert, false).Score(c, tr)
	if off.Value != nil {
		t.Fatal("disabled judge must skip")
	}
}
