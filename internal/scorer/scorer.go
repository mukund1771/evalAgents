// Package scorer grades a trajectory. Nil score values mean the scorer skipped.
package scorer

import "agenteval/internal/eval"

// Scorer grades one case trajectory.
type Scorer interface {
	Name() string
	Score(c eval.Case, t eval.Trajectory) eval.Score
}

// TrialPassed is true when at least one score applied and every applied score passed.
func TrialPassed(scores []eval.Score) bool {
	applied := 0
	for _, s := range scores {
		if !s.Applicable() {
			continue
		}
		applied++
		if !s.OK() {
			return false
		}
	}
	return applied > 0
}
