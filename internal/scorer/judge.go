package scorer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mukund1771/evalAgents/internal/eval"
)

// Completer is one chat completion. Tests stub it. The HTTP client implements it.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

type modelGraded struct {
	name     string
	metric   string
	min      float64
	complete Completer
	enabled  bool
}

// ModelGraded is the single LLM judge. enabled is false when no API key is set,
// and the score is then skipped rather than failed.
func ModelGraded(name, metric string, min float64, c Completer, enabled bool) Scorer {
	if metric == "" {
		metric = "task_completion"
	}
	return modelGraded{name: name, metric: metric, min: min, complete: c, enabled: enabled}
}

func (s modelGraded) Name() string { return s.name }

type judgeOut struct {
	Value       *float64    `json:"value"`
	Passed      *bool       `json:"passed"`
	Explanation string      `json:"explanation"`
	Evidence    []eval.Span `json:"evidence"`
}

func (s modelGraded) Score(c eval.Case, t eval.Trajectory) eval.Score {
	if !s.enabled || s.complete == nil {
		return eval.Skip(s.name, "skipped: judge disabled or OPENAI_API_KEY is unset")
	}
	sys := judgeSystem(s.metric)
	user := judgeUser(s.metric, c, t)
	raw, err := s.complete.Complete(context.Background(), sys, user)
	if err != nil {
		raw2, err2 := s.complete.Complete(context.Background(), sys, user+"\n\nThe previous call failed: "+err.Error()+"\nReturn only the JSON object.")
		if err2 != nil {
			return eval.Skip(s.name, "skipped: judge request failed")
		}
		raw = raw2
	}
	out, ok := parseJudge(raw, len(t.Steps))
	if !ok {
		raw2, err2 := s.complete.Complete(context.Background(), sys, user+"\n\nYour previous reply was not valid JSON of the required schema. Return only the JSON object.")
		if err2 != nil {
			return eval.Skip(s.name, "skipped: judge output invalid")
		}
		out, ok = parseJudge(raw2, len(t.Steps))
		if !ok {
			return eval.Skip(s.name, "skipped: judge output invalid")
		}
	}
	// The model proposes a value and evidence. Passed is computed here.
	return eval.Float(s.name, *out.Value, s.min, out.Explanation, out.Evidence)
}

func judgeSystem(metric string) string {
	return "You are a strict eval judge. Reply with one JSON object and nothing else: {\"value\": <number 0 to 1>, \"passed\": <bool>, \"explanation\": <string>, \"evidence\": [{\"step_index\": <int>, \"start\": <int>, \"end\": <int>}]}. Metric: " + metric + "."
}

func judgeUser(metric string, c eval.Case, t eval.Trajectory) string {
	rubric := "Did the agent accomplish what the user asked?"
	if metric == "faithfulness" {
		rubric = "Is every claim in the final answer grounded in tool results in the trajectory?"
	}
	b, _ := json.Marshal(t)
	return fmt.Sprintf("Rubric (%s): %s\nCase input: %s\nExpected: %s\nTrajectory: %s", metric, rubric, string(c.Input), string(c.Expected), string(b))
}

func parseJudge(raw string, nSteps int) (judgeOut, bool) {
	obj := extractJSON(raw)
	var out judgeOut
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		return out, false
	}
	if out.Value == nil || *out.Value < 0 || *out.Value > 1 {
		return out, false
	}
	if out.Passed == nil || strings.TrimSpace(out.Explanation) == "" {
		return out, false
	}
	for _, sp := range out.Evidence {
		if sp.StepIndex < 0 || sp.StepIndex >= nSteps || sp.End < sp.Start {
			return out, false
		}
	}
	if nSteps == 0 && len(out.Evidence) > 0 {
		return out, false
	}
	return out, true
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	if json.Valid([]byte(s)) {
		return s
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
