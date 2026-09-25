package scorer

import (
	"bytes"
	"encoding/json"

	"agenteval/internal/eval"
)

type placedCall struct {
	key  string
	name string
	step int
}

func callsOf(t eval.Trajectory) []placedCall {
	var out []placedCall
	for i, step := range t.Steps {
		for _, tc := range step.ToolCalls {
			out = append(out, placedCall{key: toolKey(tc), name: tc.Name, step: i})
		}
	}
	return out
}

func expectedCalls(exp eval.Expected) []placedCall {
	var out []placedCall
	for _, tc := range exp.Tools {
		out = append(out, placedCall{key: toolKey(tc), name: tc.Name, step: -1})
	}
	return out
}

func toolKey(tc eval.ToolCall) string {
	return tc.Name + "\x00" + canonicalArgs(tc.Args)
}

func canonicalArgs(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "{}"
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return string(trimmed)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(trimmed)
	}
	return string(b)
}

func finalIndex(t eval.Trajectory) int {
	for i := len(t.Steps) - 1; i >= 0; i-- {
		if t.Steps[i].Kind == "final" {
			return i
		}
	}
	if len(t.Steps) == 0 {
		return 0
	}
	return len(t.Steps) - 1
}

func stepSpan(t eval.Trajectory, idx int) eval.Span {
	if idx < 0 || idx >= len(t.Steps) {
		return eval.Span{StepIndex: idx}
	}
	return eval.Span{StepIndex: idx, Start: 0, End: len(t.Steps[idx].Content)}
}

func keys(cs []placedCall) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.key
	}
	return out
}
