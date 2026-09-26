package store

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/mukund1771/evalAgents/internal/eval"
)

func TestPassK(t *testing.T) {
	if math.Abs(PassK(2, 4, 1)-0.5) > 1e-9 {
		t.Fatalf("pass^1 = %v", PassK(2, 4, 1))
	}
	// C(2,2)/C(4,2) = 1/6
	if math.Abs(PassK(2, 4, 2)-(1.0/6.0)) > 1e-9 {
		t.Fatalf("pass^2 = %v", PassK(2, 4, 2))
	}
	if PassK(2, 4, 4) != 0 {
		t.Fatal("pass^4 should be 0")
	}
	if PassK(1, 1, 1) != 1 {
		t.Fatal("single success")
	}
}

func TestRoundTrip(t *testing.T) {
	root := t.TempDir()
	id, err := NewRunID(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	run, err := Create(root, id)
	if err != nil {
		t.Fatal(err)
	}
	one := 1.0
	passed := true
	man := Manifest{ID: id, Suite: "s", StartedAt: time.Now().UTC(), Target: "replay", Scorers: []string{"contains"}, Repeats: 1}
	rows := []ResultRow{{
		CaseID: "a",
		Scores: []eval.Score{{Name: "contains", Value: &one, Passed: &passed}},
	}}
	sum := Summarize(man, rows, []string{"a"})
	if err := run.WriteManifest(man); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteResults(rows); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteSummary(sum); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, id); err == nil {
		t.Fatal("run ids are immutable")
	}
	loaded, err := LoadSummary(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Passed || loaded.Cases[0].ID != "a" {
		t.Fatalf("summary = %#v", loaded)
	}
	gotRows, err := LoadResults(root, id)
	if err != nil || len(gotRows) != 1 {
		t.Fatalf("rows %v %v", gotRows, err)
	}
	listed, err := List(root)
	if err != nil || len(listed) != 1 || listed[0].ID != id {
		t.Fatalf("list %v %v", listed, err)
	}
	if filepath.Base(run.Dir) != id {
		t.Fatal(run.Dir)
	}
}
