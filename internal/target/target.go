// Package target runs a case or replays a recorded trajectory.
package target

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agenteval/internal/eval"
)

// Target produces a trajectory for one case.
type Target interface {
	Run(c eval.Case) (eval.Trajectory, error)
}

func cassettePath(dir, id string) (string, error) {
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid case id %q", id)
	}
	return filepath.Join(dir, id+".json"), nil
}

// LoadCassette reads {id}.json. A missing file is an error (fail closed).
func LoadCassette(dir, id string) (eval.Trajectory, error) {
	path, err := cassettePath(dir, id)
	if err != nil {
		return eval.Trajectory{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return eval.Trajectory{}, fmt.Errorf("cassette miss: %s", path)
		}
		return eval.Trajectory{}, err
	}
	var tr eval.Trajectory
	if err := json.Unmarshal(b, &tr); err != nil {
		return eval.Trajectory{}, fmt.Errorf("cassette %s: %w", path, err)
	}
	if tr.CaseID == "" {
		tr.CaseID = id
	}
	if tr.CaseID != id {
		return eval.Trajectory{}, fmt.Errorf("cassette %s: case_id %q does not match %q", path, tr.CaseID, id)
	}
	return tr, nil
}

// SaveCassette writes a trajectory JSON file, creating the directory if needed.
func SaveCassette(dir, id string, tr eval.Trajectory) error {
	path, err := cassettePath(dir, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if tr.CaseID == "" {
		tr.CaseID = id
	}
	b, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}
