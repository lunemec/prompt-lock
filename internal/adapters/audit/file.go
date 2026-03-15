package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

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
	lock     *os.File
	lockPath string
	lastHash string
	needSync bool
}

var syncAuditFile = func(f *os.File) error {
	return f.Sync()
}

func NewFileSink(path string) (*FileSink, error) {
	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o700); err != nil {
		return nil, err
	}
	lockPath := cleanPath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("acquire audit lock %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("acquire audit lock %s: %w", lockPath, err)
	}
	cleanupLock := true
	defer func() {
		if cleanupLock {
			_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
			_ = lockFile.Close()
		}
	}()

	_, statErr := os.Stat(cleanPath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	needSync := os.IsNotExist(statErr)
	f, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	last := ""
	if !needSync {
		var verifyErr error
		last, _, verifyErr = verifyFile(cleanPath, "")
		if verifyErr != nil {
			_ = f.Close()
			return nil, fmt.Errorf("verify audit log %s: %w", cleanPath, verifyErr)
		}
	}
	cleanupLock = false
	return &FileSink{f: f, lock: lockFile, lockPath: lockPath, lastHash: last, needSync: needSync}, nil
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
	if err := syncAuditFile(s.f); err != nil {
		return err
	}
	if s.needSync {
		if err := syncAuditParentDir(filepath.Dir(s.f.Name())); err != nil {
			return err
		}
		s.needSync = false
	}
	s.lastHash = base.Hash
	return nil
}

func (s *FileSink) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	if s.f != nil {
		closeErr = s.f.Close()
		s.f = nil
	}
	var unlockErr error
	if s.lock != nil {
		unlockErr = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
		closeLockErr := s.lock.Close()
		s.lock = nil
		if unlockErr == nil {
			unlockErr = closeLockErr
		}
		s.lockPath = ""
	}
	if closeErr != nil {
		return closeErr
	}
	return unlockErr
}

var tokenLikeRe = regexp.MustCompile(`(?i)\b(boot|grant|sess|lease)_[a-f0-9]{16,}\b`)
var secretKVRe = regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token)\s*[:=]\s*([^\s,;]+)`)

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
	s = secretKVRe.ReplaceAllString(s, "$1=[REDACTED]")
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

var syncAuditParentDir = func(path string) error {
	return syncDir(path)
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
