package target

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mukund1771/evalAgents/internal/eval"
)

// Cmd spawns a binary. One JSON case goes in on stdin. One JSON trajectory
// comes out on stdout. This is how a Python agent is scored without a Go port.
type Cmd struct {
	Command     []string
	Timeout     time.Duration
	WorkDir     string
	Env         []string
	CassetteDir string
	Offline     bool
	Record      bool
}

func (c Cmd) Run(cs eval.Case) (eval.Trajectory, error) {
	if c.Offline {
		if c.CassetteDir == "" {
			return eval.Trajectory{}, fmt.Errorf("offline requires a cassette directory")
		}
		return LoadCassette(c.CassetteDir, cs.ID)
	}
	tr, err := c.exec(cs)
	if err != nil {
		return tr, err
	}
	if c.Record {
		if c.CassetteDir == "" {
			return tr, fmt.Errorf("record requires a cassette directory")
		}
		if err := SaveCassette(c.CassetteDir, cs.ID, tr); err != nil {
			return tr, err
		}
	}
	return tr, nil
}

func (c Cmd) exec(cs eval.Case) (eval.Trajectory, error) {
	if len(c.Command) == 0 {
		return eval.Trajectory{}, fmt.Errorf("cmd target has an empty command")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Command[0], c.Command[1:]...)
	cmd.Dir = c.WorkDir
	if len(c.Env) > 0 {
		cmd.Env = c.Env
	}
	in, err := json.Marshal(cs)
	if err != nil {
		return eval.Trajectory{}, err
	}
	cmd.Stdin = bytes.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if len(msg) > 4000 {
			msg = msg[:4000]
		}
		if ctx.Err() == context.DeadlineExceeded {
			return eval.Trajectory{}, fmt.Errorf("command timed out after %s", timeout)
		}
		return eval.Trajectory{}, fmt.Errorf("command failed: %w: %s", err, msg)
	}
	var tr eval.Trajectory
	dec := json.NewDecoder(&stdout)
	if err := dec.Decode(&tr); err != nil {
		return eval.Trajectory{}, fmt.Errorf("command stdout is not a trajectory: %w", err)
	}
	if tr.CaseID == "" {
		tr.CaseID = cs.ID
	}
	return tr, nil
}

// Environ returns the child environment with one key overridden.
func Environ(key, value string) []string {
	prefix := key + "="
	base := os.Environ()
	out := make([]string, 0, len(base)+1)
	for _, e := range base {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	out = append(out, prefix+value)
	return out
}
