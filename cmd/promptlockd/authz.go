package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/lunemec/promptlock/internal/auth"
)

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	const p = "Bearer "
	if strings.HasPrefix(h, p) {
		return strings.TrimSpace(strings.TrimPrefix(h, p))
	}
	return ""
}

func (s *server) requireOperator(w http.ResponseWriter, r *http.Request) bool {
	if !s.authEnabled {
		return true
	}
	tok := bearerToken(r)
	if tok == "" || tok != s.authCfg.OperatorToken {
		http.Error(w, "operator auth required", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *server) requireAgentSession(w http.ResponseWriter, r *http.Request) bool {
	if !s.authEnabled {
		return true
	}
	tok := bearerToken(r)
	if tok == "" {
		http.Error(w, "agent session token required", http.StatusUnauthorized)
		return false
	}
	sess, err := s.authStore.ValidateSession(tok, s.now())
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return false
	}
	g, err := s.authStore.GetGrant(sess.GrantID)
	if err != nil {
		http.Error(w, "invalid grant", http.StatusUnauthorized)
		return false
	}
	if err := s.validateGrantActive(g); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *server) validateGrantActive(g auth.PairingGrant) error {
	n := s.now()
	if g.Revoked {
		return errors.New("grant revoked")
	}
	if !n.Before(g.IdleExpiresAt) {
		return errors.New("grant idle expired")
	}
	if !n.Before(g.AbsoluteExpiresAt) {
		return errors.New("grant absolute expired")
	}
	return nil
}
