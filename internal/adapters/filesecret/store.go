package filesecret

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Store struct {
	path string
	mu   sync.RWMutex
	data map[string]string
}

func New(path string) (*Store, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return nil, fmt.Errorf("file secret source requires non-empty file path")
	}
	s := &Store{path: p, data: map[string]string{}}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Reload() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read secret file: %w", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("parse secret file: %w", err)
	}
	s.mu.Lock()
	s.data = m
	s.mu.Unlock()
	return nil
}

func (s *Store) GetSecret(name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[strings.TrimSpace(name)]
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("secret %q not found in file source", name)
	}
	return v, nil
}
