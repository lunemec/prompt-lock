package auth

import (
	"errors"
	"sync"
	"time"
)

type BootstrapToken struct {
	Token     string
	AgentID   string
	CreatedAt time.Time
	ExpiresAt time.Time
	Used      bool
}

type PairingGrant struct {
	GrantID           string
	AgentID           string
	ContainerID       string
	CreatedAt         time.Time
	LastUsedAt        time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	Revoked           bool
}

type SessionToken struct {
	Token     string
	GrantID   string
	AgentID   string
	CreatedAt time.Time
	ExpiresAt time.Time
	Revoked   bool
}

type Store struct {
	mu        sync.RWMutex
	bootstrap map[string]BootstrapToken
	grants    map[string]PairingGrant
	sessions  map[string]SessionToken
}

func NewStore() *Store {
	return &Store{
		bootstrap: map[string]BootstrapToken{},
		grants:    map[string]PairingGrant{},
		sessions:  map[string]SessionToken{},
	}
}

func (s *Store) SaveBootstrap(t BootstrapToken) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bootstrap[t.Token] = t
}

func (s *Store) ConsumeBootstrap(token string, now time.Time) (BootstrapToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.bootstrap[token]
	if !ok {
		return BootstrapToken{}, errors.New("bootstrap token not found")
	}
	if t.Used {
		return BootstrapToken{}, errors.New("bootstrap token already used")
	}
	if !now.Before(t.ExpiresAt) {
		return BootstrapToken{}, errors.New("bootstrap token expired")
	}
	t.Used = true
	s.bootstrap[token] = t
	return t, nil
}

func (s *Store) SaveGrant(g PairingGrant) { s.mu.Lock(); defer s.mu.Unlock(); s.grants[g.GrantID] = g }

func (s *Store) GetGrant(id string) (PairingGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.grants[id]
	if !ok {
		return PairingGrant{}, errors.New("grant not found")
	}
	return g, nil
}

func (s *Store) UpdateGrant(g PairingGrant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[g.GrantID] = g
}

func (s *Store) RevokeGrant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return errors.New("grant not found")
	}
	g.Revoked = true
	s.grants[id] = g
	return nil
}

func (s *Store) SaveSession(tok SessionToken) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[tok.Token] = tok
}

func (s *Store) GetSession(token string) (SessionToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.sessions[token]
	if !ok {
		return SessionToken{}, errors.New("session not found")
	}
	return t, nil
}

func (s *Store) RevokeSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.sessions[token]
	if !ok {
		return errors.New("session not found")
	}
	t.Revoked = true
	s.sessions[token] = t
	return nil
}

func (s *Store) ValidateSession(token string, now time.Time) (SessionToken, error) {
	t, err := s.GetSession(token)
	if err != nil {
		return SessionToken{}, err
	}
	if t.Revoked {
		return SessionToken{}, errors.New("session revoked")
	}
	if !now.Before(t.ExpiresAt) {
		return SessionToken{}, errors.New("session expired")
	}
	return t, nil
}
