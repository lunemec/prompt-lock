package memory

import (
	"errors"
	"sync"

	"github.com/lunemec/promptlock/internal/core/domain"
)

type Store struct {
	mu       sync.RWMutex
	requests map[string]domain.LeaseRequest
	leases   map[string]domain.Lease
	secrets  map[string]string
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
