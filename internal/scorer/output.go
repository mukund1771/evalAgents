package scorer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mukund1771/evalAgents/internal/eval"
)

type exactMatch struct {
	name string
	min  float64
}
type contains struct {
	name string
	min  float64
}
type jsonValid struct {
	name string
	min  float64
}

func ExactMatch(name string, min float64) Scorer { return exactMatch{name, min} }
func Contains(name string, min float64) Scorer   { return contains{name, min} }
func JSONValid(name string, min float64) Scorer  { return jsonValid{name, min} }

func (s exactMatch) Name() string { return s.name }
func (s contains) Name() string   { return s.name }
func (s jsonValid) Name() string  { return s.name }

func (s exactMatch) Score(c eval.Case, t eval.Trajectory) eval.Score {
	exp, err := eval.ParseExpected(c)
	if err != nil {
		return eval.Skip(s.name, err.Error())
	}
	if exp.Exact == nil {
		return eval.Skip(s.name, "skipped: expected.exact is absent")
	}
	got := strings.TrimSpace(t.FinalOutput)
	want := strings.TrimSpace(*exp.Exact)
	ev := []eval.Span{stepSpan(t, finalIndex(t))}
	if got == want {
		return eval.Float(s.name, 1, s.min, "final output matches", ev)
	}
	return eval.Float(s.name, 0, s.min, fmt.Sprintf("got %q want %q", got, want), ev)
}

func (s contains) Score(c eval.Case, t eval.Trajectory) eval.Score {
	exp, err := eval.ParseExpected(c)
	if err != nil {
		return eval.Skip(s.name, err.Error())
	}
	if exp.Contains == "" {
		return eval.Skip(s.name, "skipped: expected.contains is absent")
	}
	fi := finalIndex(t)
	if !strings.Contains(t.FinalOutput, exp.Contains) {
		return eval.Float(s.name, 0, s.min, fmt.Sprintf("final output does not contain %q", exp.Contains), []eval.Span{stepSpan(t, fi)})
	}
	// The score reads FinalOutput, but a Span is documented as byte offsets into
	// a step's content, and those are separate fields a cmd target sets
	// independently. Locate the text in the step; if it is not there, point at
	// the whole step rather than at offsets we cannot vouch for.
	ev := []eval.Span{stepSpan(t, fi)}
	if fi >= 0 && fi < len(t.Steps) {
		if i := strings.Index(t.Steps[fi].Content, exp.Contains); i >= 0 {
			ev = []eval.Span{{StepIndex: fi, Start: i, End: i + len(exp.Contains)}}
		}
	}
	return eval.Float(s.name, 1, s.min, "final output contains the expected text", ev)
}

func (s jsonValid) Score(c eval.Case, t eval.Trajectory) eval.Score {
	exp, err := eval.ParseExpected(c)
	if err != nil {
		return eval.Skip(s.name, err.Error())
	}
	if !exp.JSON {
		return eval.Skip(s.name, "skipped: expected.json is not true")
	}
	ev := []eval.Span{stepSpan(t, finalIndex(t))}
	if json.Valid([]byte(strings.TrimSpace(t.FinalOutput))) {
		return eval.Float(s.name, 1, s.min, "final output is valid JSON", ev)
	}
	return eval.Float(s.name, 0, s.min, "final output is not valid JSON", ev)
}
