// Package store writes immutable JSONL runs under .agenteval/runs/<id>/.
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"agenteval/internal/eval"
)

// Manifest describes one immutable run.
type Manifest struct {
	ID        string    `json:"id"`
	Suite     string    `json:"suite"`
	StartedAt time.Time `json:"started_at"`
	Target    string    `json:"target"`
	Scorers   []string  `json:"scorers"`
	Repeats   int       `json:"repeats"`
	Filter    string    `json:"filter,omitempty"`
	Matrix    string    `json:"matrix,omitempty"`
	Command   []string  `json:"command,omitempty"`
}

// ResultRow is one case execution (one repeat).
type ResultRow struct {
	CaseID     string          `json:"case_id"`
	Repeat     int             `json:"repeat"`
	Trajectory eval.Trajectory `json:"trajectory"`
	Scores     []eval.Score    `json:"scores"`
	Error      string          `json:"error,omitempty"`
}

// CaseSummary aggregates repeats for one case.
type CaseSummary struct {
	ID        string             `json:"id"`
	Passed    bool               `json:"passed"`
	Pass1     float64            `json:"pass_1"`
	PassK     float64            `json:"pass_k"`
	Successes int                `json:"successes"`
	Trials    int                `json:"trials"`
	Means     map[string]float64 `json:"means,omitempty"`
	Failed    []string           `json:"failed,omitempty"`
}

// ScorerAggregate is the mean of non-skipped values for one scorer.
type ScorerAggregate struct {
	Name     string  `json:"name"`
	Mean     float64 `json:"mean"`
	Count    int     `json:"count"`
	Passes   int     `json:"passes"`
	Skipped  int     `json:"skipped"`
	HasValue bool    `json:"-"`
}

// Summary is the comparable artifact for diff and the human table.
type Summary struct {
	RunID   string            `json:"run_id"`
	Suite   string            `json:"suite"`
	Passed  bool              `json:"passed"`
	Pass1   float64           `json:"pass_1"`
	PassK   float64           `json:"pass_k"`
	Repeats int               `json:"repeats"`
	Matrix  string            `json:"matrix,omitempty"`
	Cases   []CaseSummary     `json:"cases"`
	Scorers []ScorerAggregate `json:"scorers"`
}

// NewRunID returns a sortable id: UTC timestamp plus 6 hex chars.
func NewRunID(now time.Time) (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:]), nil
}

// Run is an open run directory.
type Run struct {
	Dir string
	ID  string
}

// Create makes a new run directory. It refuses to reuse an id.
func Create(root, id string) (*Run, error) {
	dir := filepath.Join(root, "runs", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// MkdirAll succeeds if the directory exists. Refuse a second write.
	marker := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(marker); err == nil {
		return nil, fmt.Errorf("run %s already exists", id)
	}
	return &Run{Dir: dir, ID: id}, nil
}

// WriteManifest writes manifest.json.
func (r *Run) WriteManifest(m Manifest) error {
	return writeJSON(filepath.Join(r.Dir, "manifest.json"), m)
}

// WriteResults writes results.jsonl, one row per line.
func (r *Run) WriteResults(rows []ResultRow) error {
	f, err := os.Create(filepath.Join(r.Dir, "results.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return w.Flush()
}

// WriteSummary writes summary.json.
func (r *Run) WriteSummary(s Summary) error {
	return writeJSON(filepath.Join(r.Dir, "summary.json"), s)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

// LoadSummary reads summary.json for a run id.
func LoadSummary(root, id string) (Summary, error) {
	var s Summary
	b, err := os.ReadFile(filepath.Join(root, "runs", id, "summary.json"))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

// LoadManifest reads manifest.json.
func LoadManifest(root, id string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(root, "runs", id, "manifest.json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}

// LoadResults reads results.jsonl.
func LoadResults(root, id string) ([]ResultRow, error) {
	f, err := os.Open(filepath.Join(root, "runs", id, "results.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []ResultRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var row ResultRow
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, sc.Err()
}

// List returns manifests, newest first.
func List(root string) ([]Manifest, error) {
	entries, err := os.ReadDir(filepath.Join(root, "runs"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := LoadManifest(root, e.Name())
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}
