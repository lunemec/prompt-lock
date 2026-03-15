package audit

import (
	"errors"
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

func TestVerifyFileRejectsNonCanonicalStoredBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ports.AuditEvent{Event: "e1", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(b), `"hash":"`, `"extra":"tamper","hash":"`, 1)
	if err := os.WriteFile(p, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := VerifyFile(p); err == nil {
		t.Fatalf("expected verify failure for non-canonical stored bytes")
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

func TestVerifyFileAnchoredAllowsAppendAfterCheckpoint(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ports.AuditEvent{Event: "e1", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	firstHash, _, err := VerifyFile(p)
	if err != nil {
		t.Fatalf("verify after first record: %v", err)
	}
	if err := s.Write(ports.AuditEvent{Event: "e2", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	lastHash, count, err := VerifyFileAnchored(p, firstHash)
	if err != nil {
		t.Fatalf("expected anchored verify success after append, got %v", err)
	}
	if count != 2 {
		t.Fatalf("record count = %d, want 2", count)
	}
	if lastHash == firstHash {
		t.Fatalf("expected appended chain to advance past checkpoint hash")
	}
}

func TestVerifyFileAnchoredRejectsMissingCheckpoint(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ports.AuditEvent{Event: "e1", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyFileAnchored(p, "missing-hash"); err == nil {
		t.Fatalf("expected missing checkpoint hash to fail")
	}
}

func TestVerifyFileRejectsUnknownFieldTamper(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ports.AuditEvent{Event: "e1", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(b), `,"hash":`, `,"forged":"x","hash":`, 1)
	if err := os.WriteFile(p, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := VerifyFile(p); err == nil {
		t.Fatalf("expected unknown field tamper to fail verification")
	}
}

func TestWriteCheckpointFailsWhenParentDirSyncFails(t *testing.T) {
	orig := syncCheckpointParentDir
	t.Cleanup(func() { syncCheckpointParentDir = orig })
	syncCheckpointParentDir = func(string) error {
		return errors.New("dir sync failed")
	}

	if err := WriteCheckpoint(filepath.Join(t.TempDir(), "checkpoint.txt"), "abc123"); err == nil {
		t.Fatalf("expected parent dir sync failure")
	}
}
