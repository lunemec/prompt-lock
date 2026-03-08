package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/core/ports"
)

type rec struct {
	Event    ports.AuditEvent `json:"event"`
	PrevHash string           `json:"prev_hash"`
	Hash     string           `json:"hash"`
}

func TestHashChain(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.Write(ports.AuditEvent{Event: "e1", Timestamp: time.Now().UTC()})
	_ = s.Write(ports.AuditEvent{Event: "e2", Timestamp: time.Now().UTC()})

	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var all []rec
	for scanner.Scan() {
		var r rec
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d", len(all))
	}
	if all[0].Hash == "" || all[1].Hash == "" {
		t.Fatalf("hash must be present")
	}
	if all[1].PrevHash != all[0].Hash {
		t.Fatalf("expected prev hash linkage")
	}
}

func TestAuditSanitizesTokenFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	e := ports.AuditEvent{
		Event:      "x",
		Timestamp:  time.Now().UTC(),
		LeaseToken: "lease_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Metadata: map[string]string{
			"note": "Bearer sess_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}
	if err := s.Write(e); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatalf("expected audit record")
	}
	var r rec
	if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Event.LeaseToken == "" || r.Event.LeaseToken == "lease_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected lease token to be sanitized, got %q", r.Event.LeaseToken)
	}
	if got := r.Event.Metadata["note"]; got == "Bearer sess_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || got == "" {
		t.Fatalf("expected metadata note sanitized, got %q", got)
	}
}
