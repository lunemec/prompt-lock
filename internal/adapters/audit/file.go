package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
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

	event = sanitizeAuditEvent(event)
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

var tokenLikeRe = regexp.MustCompile(`(?i)\b(boot|grant|sess|lease)_[a-f0-9]{16,}\b`)

func sanitizeAuditEvent(e ports.AuditEvent) ports.AuditEvent {
	if e.LeaseToken != "" {
		e.LeaseToken = "tokhash_" + shortHash(e.LeaseToken)
	}
	if e.Metadata != nil {
		out := make(map[string]string, len(e.Metadata))
		for k, v := range e.Metadata {
			out[k] = sanitizeText(v)
		}
		e.Metadata = out
	}
	e.ActorID = sanitizeText(e.ActorID)
	e.AgentID = sanitizeText(e.AgentID)
	e.TaskID = sanitizeText(e.TaskID)
	e.RequestID = sanitizeText(e.RequestID)
	e.Secret = sanitizeText(e.Secret)
	return e
}

func sanitizeText(in string) string {
	s := in
	s = tokenLikeRe.ReplaceAllStringFunc(s, func(tok string) string {
		return "tokhash_" + shortHash(tok)
	})
	lower := strings.ToLower(s)
	if strings.Contains(lower, "bearer ") {
		parts := strings.SplitN(s, " ", 2)
		if len(parts) == 2 {
			return parts[0] + " [REDACTED]"
		}
		return "[REDACTED]"
	}
	return s
}

func shortHash(in string) string {
	h := sha256.Sum256([]byte(in))
	full := hex.EncodeToString(h[:])
	if len(full) > 16 {
		return full[:16]
	}
	return full
}

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
