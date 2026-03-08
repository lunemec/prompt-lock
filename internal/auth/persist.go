package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type persistedState struct {
	Bootstrap map[string]BootstrapToken `json:"bootstrap"`
	Grants    map[string]PairingGrant   `json:"grants"`
	Sessions  map[string]SessionToken   `json:"sessions"`
}

func (s *Store) SaveToFile(path string) error {
	s.mu.RLock()
	state := persistedState{
		Bootstrap: make(map[string]BootstrapToken, len(s.bootstrap)),
		Grants:    make(map[string]PairingGrant, len(s.grants)),
		Sessions:  make(map[string]SessionToken, len(s.sessions)),
	}
	for k, v := range s.bootstrap {
		state.Bootstrap[k] = v
	}
	for k, v := range s.grants {
		state.Grants[k] = v
	}
	for k, v := range s.sessions {
		state.Sessions[k] = v
	}
	s.mu.RUnlock()

	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

func (s *Store) LoadFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var state persistedState
	if err := json.Unmarshal(b, &state); err != nil {
		return fmt.Errorf("parse auth store: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.Bootstrap != nil {
		s.bootstrap = state.Bootstrap
	}
	if state.Grants != nil {
		s.grants = state.Grants
	}
	if state.Sessions != nil {
		s.sessions = state.Sessions
	}
	return nil
}
