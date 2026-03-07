package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/lunemec/promptlock/internal/core/ports"
)

type auditRecord struct {
	Event    ports.AuditEvent `json:"event"`
	PrevHash string           `json:"prev_hash"`
	Hash     string           `json:"hash"`
}

type FileSink struct {
	mu       sync.Mutex
	f        *os.File
	lastHash string
}

func NewFileSink(path string) (*FileSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	last := ""
	if prev, err := readLastHash(path); err == nil {
		last = prev
	}
	return &FileSink{f: f, lastHash: last}, nil
}

func (s *FileSink) Write(event ports.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	base := auditRecord{Event: event, PrevHash: s.lastHash}
	tmp := base
	tmp.Hash = ""
	b, err := json.Marshal(tmp)
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	base.Hash = hex.EncodeToString(h[:])

	enc := json.NewEncoder(s.f)
	if err := enc.Encode(base); err != nil {
		return err
	}
	s.lastHash = base.Hash
	return nil
}

func (s *FileSink) Close() error { return s.f.Close() }

func readLastHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	lastLine := ""
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" {
			lastLine = line
		}
	}
	if lastLine == "" {
		return "", nil
	}
	var rec auditRecord
	if err := json.Unmarshal([]byte(lastLine), &rec); err != nil {
		return "", err
	}
	return rec.Hash, nil
}
