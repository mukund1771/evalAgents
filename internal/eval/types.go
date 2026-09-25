// Package eval holds the contracts shared by targets, scorers, and the store.
// A trajectory is the thing being scored. A final string is only one field on it.
package eval

import "encoding/json"

// Case is one eval example. Expected is interpreted by each scorer; a scorer
// that does not find its field skips (nil score) instead of failing.
type Case struct {
	ID       string          `json:"id" yaml:"id"`
	Input    json.RawMessage `json:"input" yaml:"input"`
	Expected json.RawMessage `json:"expected,omitempty" yaml:"expected,omitempty"`
	Tags     []string        `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// ToolCall is one invocation the agent requested.
type ToolCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Step is one turn in a trajectory.
// Kind is user, assistant, tool_result, or final.
type Step struct {
	Kind      string     `json:"kind"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	CallID    string     `json:"call_id,omitempty"`
	IsError   bool       `json:"is_error,omitempty"`
}

// Metrics are recorded for inspection. Deterministic scorers do not read them.
type Metrics struct {
	Tokens    int     `json:"tokens,omitempty"`
	Cost      float64 `json:"cost,omitempty"`
	LatencyMS float64 `json:"latency_ms,omitempty"`
}

// Trajectory is the primary artifact. Agent evals score this, not a final string.
type Trajectory struct {
	CaseID      string  `json:"case_id"`
	Steps       []Step  `json:"steps"`
	FinalOutput string  `json:"final_output"`
	Metrics     Metrics `json:"metrics"`
}

// Span points at the transcript region that caused a score.
// Start and End are byte offsets into that step's content.
type Span struct {
	StepIndex int `json:"step_index"`
	Start     int `json:"start"`
	End       int `json:"end"`
}

// Score is one scorer's judgment.
// Value nil means skipped, not zero. Passed is set only when Value is set.
type Score struct {
	Name        string   `json:"name"`
	Value       *float64 `json:"value"`
	Passed      *bool    `json:"passed,omitempty"`
	Explanation string   `json:"explanation,omitempty"`
	Evidence    []Span   `json:"evidence,omitempty"`
}

// Expected is the subset of Case.Expected that built-in scorers understand.
type Expected struct {
	Exact    *string    `json:"exact,omitempty"`
	Contains string     `json:"contains,omitempty"`
	JSON     bool       `json:"json,omitempty"`
	Tools    []ToolCall `json:"tools,omitempty"`
	MaxSteps *int       `json:"max_steps,omitempty"`
}

// ParseExpected decodes Case.Expected. Missing expected yields a zero struct.
func ParseExpected(c Case) (Expected, error) {
	if len(c.Expected) == 0 || string(c.Expected) == "null" {
		return Expected{}, nil
	}
	var exp Expected
	if err := json.Unmarshal(c.Expected, &exp); err != nil {
		return Expected{}, err
	}
	return exp, nil
}
