package main

import (
	"crypto/subtle"
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

func (s *server) requireOperator(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if !s.authEnabled {
		return withActor(r, "operator", "local-operator"), true
	}
	if !s.enforceAuthRateLimit(w, r, "operator") {
		return r, false
	}
	tok := bearerToken(r)
	expected := s.authCfg.OperatorToken
	if tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(expected)) != 1 {
		s.recordAuthFailure(r, "operator", "invalid_operator_token")
		writeMappedError(w, ErrUnauthorized, "operator auth required")
		return r, false
	}
	return withActor(r, "operator", "token-operator"), true
}

func (s *server) requireAgentSession(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if !s.authEnabled {
		return withActor(r, "agent", "unauth-agent"), true
	}
	if !s.enforceAuthRateLimit(w, r, "agent") {
		return r, false
	}
	tok := bearerToken(r)
	if tok == "" {
		s.recordAuthFailure(r, "agent", "missing_session_token")
		writeMappedError(w, ErrUnauthorized, "agent session token required")
		return r, false
	}
	sess, err := s.authStore.ValidateSession(tok, s.now())
	if err != nil {
		s.recordAuthFailure(r, "agent", "invalid_session_token")
		writeMappedError(w, ErrUnauthorized, "invalid session")
		return r, false
	}
	g, err := s.authStore.GetGrant(sess.GrantID)
	if err != nil {
		s.recordAuthFailure(r, "agent", "invalid_grant")
		writeMappedError(w, ErrUnauthorized, "invalid grant")
		return r, false
	}
	if err := s.validateGrantActive(g); err != nil {
		s.recordAuthFailure(r, "agent", "inactive_grant")
		writeMappedError(w, ErrUnauthorized, err.Error())
		return r, false
	}
	return withActor(r, "agent", sess.AgentID), true
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
