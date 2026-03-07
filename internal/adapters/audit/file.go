package audit

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/lunemec/promptlock/internal/core/ports"
)

type FileSink struct {
	mu sync.Mutex
	f  *os.File
}

func NewFileSink(path string) (*FileSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &FileSink{f: f}, nil
}

func (s *FileSink) Write(event ports.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	enc := json.NewEncoder(s.f)
	return enc.Encode(event)
}

func (s *FileSink) Close() error { return s.f.Close() }
