package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/core/ports"
)

func TestVerifyFileDetectsTamper(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ports.AuditEvent{Event: "e1", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ports.AuditEvent{Event: "e2", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	if _, _, err := VerifyFile(p); err != nil {
		t.Fatalf("expected verify success, got %v", err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(b), "\"event\":\"e2\"", "\"event\":\"E2_TAMPER\"", 1)
	if err := os.WriteFile(p, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyFile(p); err == nil {
		t.Fatalf("expected verify failure on tamper")
	}
}

func TestCheckpointRoundTrip(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "checkpoint.txt")
	if err := WriteCheckpoint(p, "abc123"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCheckpoint(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc123" {
		t.Fatalf("unexpected checkpoint: %q", got)
	}
}
