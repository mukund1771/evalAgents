package scorer

import (
	"fmt"
	"strings"

	"github.com/mukund1771/evalAgents/internal/eval"
)

type toolExact struct {
	name string
	min  float64
}
type toolSubseq struct {
	name string
	min  float64
}
type toolF1 struct {
	name string
	min  float64
}
type toolsOK struct {
	name string
	min  float64
}
type stepsWithin struct {
	name string
	max  int
	min  float64
}
type noLoop struct {
	name       string
	maxRepeats int
	min        float64
}

func ToolTrajectoryExact(name string, min float64) Scorer { return toolExact{name, min} }
func ToolTrajectorySubsequence(name string, min float64) Scorer {
	return toolSubseq{name, min}
}
func ToolCallF1(name string, min float64) Scorer     { return toolF1{name, min} }
func ToolsSucceeded(name string, min float64) Scorer { return toolsOK{name, min} }
func StepsWithin(name string, max int, min float64) Scorer {
	return stepsWithin{name, max, min}
}
func NoLoop(name string, maxRepeats int, min float64) Scorer {
	if maxRepeats <= 0 {
		maxRepeats = 2
	}
	return noLoop{name, maxRepeats, min}
}

func (s toolExact) Name() string   { return s.name }
func (s toolSubseq) Name() string  { return s.name }
func (s toolF1) Name() string      { return s.name }
func (s toolsOK) Name() string     { return s.name }
func (s stepsWithin) Name() string { return s.name }
func (s noLoop) Name() string      { return s.name }

func (s toolExact) Score(c eval.Case, t eval.Trajectory) eval.Score {
	exp, err := eval.ParseExpected(c)
	if err != nil {
		return eval.Skip(s.name, err.Error())
	}
	if len(exp.Tools) == 0 {
		return eval.Skip(s.name, "skipped: expected.tools is absent")
	}
	got, want := callsOf(t), expectedCalls(exp)
	gk, wk := keys(got), keys(want)
	if len(gk) == len(wk) {
		match := true
		for i := range gk {
			if gk[i] != wk[i] {
				match = false
				break
			}
		}
		if match {
			return eval.Float(s.name, 1, s.min, "tool sequence matches", nil)
		}
	}
	pos := 0
	limit := len(gk)
	if len(wk) < limit {
		limit = len(wk)
	}
	for pos < limit && gk[pos] == wk[pos] {
		pos++
	}
	evIdx := 0
	if pos < len(got) {
		evIdx = got[pos].step
	} else if len(got) > 0 {
		evIdx = got[len(got)-1].step
	}
	gotName, wantName := "<none>", "<none>"
	if pos < len(got) {
		gotName = got[pos].name
	}
	if pos < len(want) {
		wantName = want[pos].name
	}
	msg := fmt.Sprintf("tool sequence mismatch at position %d: got %s want %s", pos, gotName, wantName)
	return eval.Float(s.name, 0, s.min, msg, []eval.Span{stepSpan(t, evIdx)})
}

func (s toolSubseq) Score(c eval.Case, t eval.Trajectory) eval.Score {
	exp, err := eval.ParseExpected(c)
	if err != nil {
		return eval.Skip(s.name, err.Error())
	}
	if len(exp.Tools) == 0 {
		return eval.Skip(s.name, "skipped: expected.tools is absent")
	}
	got, want := callsOf(t), expectedCalls(exp)
	n := lcs(keys(want), keys(got))
	value := float64(n) / float64(len(want))
	msg := fmt.Sprintf("matched %d of %d expected tool calls in order", n, len(want))
	var ev []eval.Span
	if value < 1 && len(got) > 0 {
		ev = []eval.Span{stepSpan(t, got[len(got)-1].step)}
	}
	return eval.Float(s.name, value, s.min, msg, ev)
}

func (s toolF1) Score(c eval.Case, t eval.Trajectory) eval.Score {
	exp, err := eval.ParseExpected(c)
	if err != nil {
		return eval.Skip(s.name, err.Error())
	}
	if len(exp.Tools) == 0 {
		return eval.Skip(s.name, "skipped: expected.tools is absent")
	}
	got, want := callsOf(t), expectedCalls(exp)
	value, p, r := multisetF1(keys(want), keys(got))
	msg := fmt.Sprintf("precision %.2f recall %.2f", p, r)
	var ev []eval.Span
	if value < 1 {
		ev = unmatchedEvidence(t, want, got)
	}
	return eval.Float(s.name, value, s.min, msg, ev)
}

func (s toolsOK) Score(_ eval.Case, t eval.Trajectory) eval.Score {
	var bad []string
	var ev []eval.Span
	for i, step := range t.Steps {
		if !step.IsError {
			continue
		}
		if step.Kind != "tool_result" && step.Kind != "tool" && step.CallID == "" {
			continue
		}
		label := step.CallID
		if label == "" {
			label = step.Kind
		}
		bad = append(bad, fmt.Sprintf("step %d (%s)", i, label))
		ev = append(ev, stepSpan(t, i))
	}
	if len(bad) == 0 {
		return eval.Float(s.name, 1, s.min, "no tool step errored", nil)
	}
	return eval.Float(s.name, 0, s.min, "tool error: "+strings.Join(bad, ", "), ev)
}

func (s stepsWithin) Score(c eval.Case, t eval.Trajectory) eval.Score {
	max := s.max
	if exp, err := eval.ParseExpected(c); err == nil && exp.MaxSteps != nil {
		max = *exp.MaxSteps
	}
	n := len(t.Steps)
	if n <= max {
		return eval.Float(s.name, 1, s.min, fmt.Sprintf("%d steps within budget %d", n, max), nil)
	}
	idx := max
	if idx >= n {
		idx = n - 1
	}
	return eval.Float(s.name, 0, s.min, fmt.Sprintf("%d steps exceeds budget %d", n, max), []eval.Span{stepSpan(t, idx)})
}

func (s noLoop) Score(_ eval.Case, t eval.Trajectory) eval.Score {
	got := callsOf(t)
	run := 1
	for i := 1; i < len(got); i++ {
		if got[i].key == got[i-1].key {
			run++
			if run > s.maxRepeats {
				msg := fmt.Sprintf("%s repeated %d times (limit %d)", got[i].name, run, s.maxRepeats)
				return eval.Float(s.name, 0, s.min, msg, []eval.Span{stepSpan(t, got[i].step)})
			}
		} else {
			run = 1
		}
	}
	return eval.Float(s.name, 1, s.min, "no repeated tool loop", nil)
}

func lcs(a, b []string) int {
	m := len(b)
	dp := make([]int, m+1)
	for i := 1; i <= len(a); i++ {
		prev := 0
		for j := 1; j <= m; j++ {
			cur := dp[j]
			if a[i-1] == b[j-1] {
				dp[j] = prev + 1
			} else if dp[j-1] > dp[j] {
				dp[j] = dp[j-1]
			}
			prev = cur
		}
	}
	return dp[m]
}

func multisetF1(expected, actual []string) (float64, float64, float64) {
	ec := map[string]int{}
	ac := map[string]int{}
	for _, k := range expected {
		ec[k]++
	}
	for _, k := range actual {
		ac[k]++
	}
	matched := 0
	for k, n := range ec {
		if ac[k] < n {
			matched += ac[k]
		} else {
			matched += n
		}
	}
	var p, r float64
	if len(actual) > 0 {
		p = float64(matched) / float64(len(actual))
	}
	if len(expected) > 0 {
		r = float64(matched) / float64(len(expected))
	}
	if p+r == 0 {
		return 0, p, r
	}
	return 2 * p * r / (p + r), p, r
}

func unmatchedEvidence(t eval.Trajectory, want, got []placedCall) []eval.Span {
	used := map[int]bool{}
	wc := map[string]int{}
	for _, w := range want {
		wc[w.key]++
	}
	for i, g := range got {
		if wc[g.key] > 0 {
			wc[g.key]--
			used[i] = true
		}
	}
	var ev []eval.Span
	for i, g := range got {
		if !used[i] {
			ev = append(ev, stepSpan(t, g.step))
		}
	}
	if len(ev) == 0 && len(got) > 0 {
		ev = []eval.Span{stepSpan(t, got[len(got)-1].step)}
	}
	if len(ev) == 0 && len(t.Steps) > 0 {
		ev = []eval.Span{stepSpan(t, finalIndex(t))}
	}
	return ev
}
