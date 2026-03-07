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
