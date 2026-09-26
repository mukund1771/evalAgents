package target

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mukund1771/evalAgents/internal/eval"
)

func TestReplayMiss(t *testing.T) {
	_, err := Replay{Dir: t.TempDir()}.Run(eval.Case{ID: "missing"})
	if err == nil || !contains(err.Error(), "cassette miss") {
		t.Fatalf("err = %v", err)
	}
}

func TestCmdRecordAndOffline(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.sh")
	body := "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '{\"case_id\":\"a\",\"steps\":[],\"final_output\":\"hi\",\"metrics\":{}}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cassettes := filepath.Join(dir, "cassettes")
	cmd := Cmd{
		Command:     []string{script},
		Timeout:     5 * time.Second,
		CassetteDir: cassettes,
		Record:      true,
	}
	tr, err := cmd.Run(eval.Case{ID: "a", Input: []byte(`{"user":"hi"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if tr.FinalOutput != "hi" {
		t.Fatalf("final %q", tr.FinalOutput)
	}
	off := Cmd{Command: []string{script}, CassetteDir: cassettes, Offline: true}
	got, err := off.Run(eval.Case{ID: "a"})
	if err != nil || got.FinalOutput != "hi" {
		t.Fatalf("offline %v %v", got.FinalOutput, err)
	}
	_, err = off.Run(eval.Case{ID: "missing"})
	if err == nil || !contains(err.Error(), "cassette miss") {
		t.Fatalf("offline miss = %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
