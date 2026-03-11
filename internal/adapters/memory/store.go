package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/lunemec/promptlock/internal/core/domain"
)

type Store struct {
	mu       sync.RWMutex
	requests map[string]domain.LeaseRequest
	leases   map[string]domain.Lease
	secrets  map[string]string
}

type persistedState struct {
	Requests map[string]domain.LeaseRequest `json:"requests"`
	Leases   map[string]domain.Lease        `json:"leases"`
}

var syncParentDir = func(path string) error {
	dir := filepath.Dir(path)
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open parent dir %s: %w", dir, err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("sync parent dir %s: %w", dir, err)
	}
	return nil
}

func NewStore() *Store {
	return &Store{
		requests: map[string]domain.LeaseRequest{},
		leases:   map[string]domain.Lease{},
		secrets:  map[string]string{},
	}
}

func (s *Store) SaveRequest(req domain.LeaseRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[req.ID] = req
	return nil
}

func (s *Store) GetRequest(id string) (domain.LeaseRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.requests[id]
	if !ok {
		return domain.LeaseRequest{}, errors.New("request not found")
	}
	return r, nil
}

func (s *Store) UpdateRequest(req domain.LeaseRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[req.ID] = req
	return nil
}

func (s *Store) ListPendingRequests() ([]domain.LeaseRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.LeaseRequest, 0)
	for _, r := range s.requests {
		if r.Status == domain.RequestPending {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) SaveLease(lease domain.Lease) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[lease.Token] = lease
	return nil
}

func (s *Store) GetLease(token string) (domain.Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.leases[token]
	if !ok {
		return domain.Lease{}, errors.New("lease not found")
	}
	return l, nil
}

func (s *Store) GetLeaseByRequestID(requestID string) (domain.Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.leases {
		if l.RequestID == requestID {
			return l, nil
		}
	}
	return domain.Lease{}, errors.New("lease not found for request")
}

func (s *Store) ListLeases() ([]domain.Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		out = append(out, cloneLease(lease))
	}
	return out, nil
}

func (s *Store) GetSecret(name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.secrets[name]
	if !ok {
		return "", errors.New("secret not found")
	}
	return v, nil
}

func (s *Store) SetSecret(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[name] = value
}

func (s *Store) SaveStateToFile(path string) error {
	s.mu.RLock()
	state := persistedState{
		Requests: make(map[string]domain.LeaseRequest, len(s.requests)),
		Leases:   make(map[string]domain.Lease, len(s.leases)),
	}
	for k, v := range s.requests {
		state.Requests[k] = cloneLeaseRequest(v)
	}
	for k, v := range s.leases {
		state.Leases[k] = cloneLease(v)
	}
	s.mu.RUnlock()

	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(b); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if err := syncParentDir(path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (s *Store) LoadStateFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}

	var state persistedState
	if err := json.Unmarshal(b, &state); err != nil {
		return fmt.Errorf("parse memory store: %w", err)
	}

	requests := make(map[string]domain.LeaseRequest)
	for k, v := range state.Requests {
		requests[k] = cloneLeaseRequest(v)
	}
	leases := make(map[string]domain.Lease)
	for k, v := range state.Leases {
		leases[k] = cloneLease(v)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = requests
	s.leases = leases
	if s.secrets == nil {
		s.secrets = map[string]string{}
	}
	return nil
}

func cloneLeaseRequest(req domain.LeaseRequest) domain.LeaseRequest {
	if req.Secrets == nil {
		return req
	}
	cp := req
	cp.Secrets = append([]string(nil), req.Secrets...)
	return cp
}

func cloneLease(lease domain.Lease) domain.Lease {
	if lease.Secrets == nil {
		return lease
	}
	cp := lease
	cp.Secrets = append([]string(nil), lease.Secrets...)
	return cp
}
