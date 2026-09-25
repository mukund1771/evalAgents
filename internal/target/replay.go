package target

import "agenteval/internal/eval"

// Replay scores trajectories recorded earlier. It does not execute an agent.
type Replay struct {
	Dir string
}

func (r Replay) Run(c eval.Case) (eval.Trajectory, error) {
	return LoadCassette(r.Dir, c.ID)
}
