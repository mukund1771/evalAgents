package scorer

import (
	"fmt"

	"github.com/mukund1771/evalAgents/internal/suite"
)

// Build turns suite scorer specs into scorers.
// tool_call_accuracy is an alias for tool_call_f1, matching Lyzr's tool accuracy metric.
func Build(specs []suite.ScorerSpec, judge Completer, judgeEnabled bool) ([]Scorer, error) {
	out := make([]Scorer, 0, len(specs))
	for _, spec := range specs {
		name := spec.Name()
		min := spec.MinScore()
		kind := spec.Kind
		if kind == "tool_call_accuracy" {
			kind = "tool_call_f1"
		}
		var s Scorer
		switch kind {
		case "exact_match":
			s = ExactMatch(name, min)
		case "contains":
			s = Contains(name, min)
		case "json_valid":
			s = JSONValid(name, min)
		case "tool_trajectory_exact":
			s = ToolTrajectoryExact(name, min)
		case "tool_trajectory_subsequence":
			s = ToolTrajectorySubsequence(name, min)
		case "tool_call_f1":
			s = ToolCallF1(name, min)
		case "tools_succeeded":
			s = ToolsSucceeded(name, min)
		case "steps_within":
			s = StepsWithin(name, spec.Max, min)
		case "no_loop":
			s = NoLoop(name, spec.MaxRepeats, min)
		case "model_graded":
			s = ModelGraded(name, spec.Metric, min, judge, judgeEnabled)
		default:
			return nil, fmt.Errorf("unknown scorer %q", spec.Kind)
		}
		out = append(out, s)
	}
	return out, nil
}
