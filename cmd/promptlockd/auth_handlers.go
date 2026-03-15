package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/lunemec/promptlock/internal/app"
)

type bootstrapCreateReq struct {
	AgentID     string `json:"agent_id"`
	ContainerID string `json:"container_id"`
}

type bootstrapConsumeReq struct {
	Token       string `json:"token"`
	ContainerID string `json:"container_id"`
}

type mintSessionReq struct {
	GrantID string `json:"grant_id"`
}

type revokeReq struct {
	GrantID      string `json:"grant_id,omitempty"`
	SessionToken string `json:"session_token,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

func (s *server) handleAuthBootstrapCreate(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authEnabled {
		writeMappedError(w, ErrBadRequest, "auth disabled")
		return
	}
	if !s.requireDurabilityReady(w) {
		return
	}
	var req bootstrapCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	at, aid := actorFromRequest(r)
	t, err := s.authLifecycleSvc().CreateBootstrap(req.AgentID, req.ContainerID, at, aid)
	if err != nil {
		if errors.Is(err, ErrDurabilityClosed) || errors.Is(err, app.ErrAuditUnavailable) {
			writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
			return
		}
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"bootstrap_token": t.Token, "expires_at": t.ExpiresAt})
}

func (s *server) handleAuthPairComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.enforceAuthRateLimit(w, r, "auth_pair_complete") {
		return
	}
	if !s.authEnabled {
		writeMappedError(w, ErrBadRequest, "auth disabled")
		return
	}
	if !s.requireDurabilityReady(w) {
		return
	}
	var req bootstrapConsumeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	g, err := s.authLifecycleSvc().CompletePairing(req.Token, req.ContainerID)
	if err != nil {
		if errors.Is(err, ErrDurabilityClosed) || errors.Is(err, app.ErrAuditUnavailable) {
			writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
			return
		}
		s.recordAuthFailure(r, "auth_pair_complete", err.Error())
		if err.Error() == "token and container_id are required" {
			writeMappedError(w, ErrBadRequest, err.Error())
			return
		}
		writeMappedError(w, ErrForbidden, err.Error())
		return
	}
	writeJSON(w, map[string]any{"grant_id": g.GrantID, "idle_expires_at": g.IdleExpiresAt, "absolute_expires_at": g.AbsoluteExpiresAt})
}

func (s *server) handleAuthSessionMint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.enforceAuthRateLimit(w, r, "auth_session_mint") {
		return
	}
	if !s.authEnabled {
		writeMappedError(w, ErrBadRequest, "auth disabled")
		return
	}
	if !s.requireDurabilityReady(w) {
		return
	}
	var req mintSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	st, err := s.authLifecycleSvc().MintSession(req.GrantID)
	if err != nil {
		if errors.Is(err, ErrDurabilityClosed) || errors.Is(err, app.ErrAuditUnavailable) {
			writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
			return
		}
		s.recordAuthFailure(r, "auth_session_mint", err.Error())
		switch err.Error() {
		case "grant not found":
			writeMappedError(w, ErrNotFound, err.Error())
		case "grant expired or revoked":
			writeMappedError(w, ErrForbidden, err.Error())
		default:
			writeMappedError(w, ErrBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, map[string]any{"session_token": st.Token, "expires_at": st.ExpiresAt})
}

func (s *server) handleAuthRevoke(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authEnabled {
		writeMappedError(w, ErrBadRequest, "auth disabled")
		return
	}
	if !s.requireDurabilityReady(w) {
		return
	}
	var req revokeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	sessionToken := req.SessionToken
	if sessionToken == "" {
		sessionToken = req.SessionID
	}
	at, aid := actorFromRequest(r)
	if err := s.authLifecycleSvc().Revoke(req.GrantID, sessionToken, at, aid); err != nil {
		if errors.Is(err, ErrDurabilityClosed) || errors.Is(err, app.ErrAuditUnavailable) {
			writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
			return
		}
		if err.Error() == "grant_id or session_token is required" {
			writeMappedError(w, ErrBadRequest, err.Error())
			return
		}
		writeMappedError(w, ErrNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"status": "revoked"})
}

func (s *server) authLifecycleSvc() app.AuthLifecycle {
	return app.AuthLifecycle{
		Store: s.authStore,
		Audit: s.svc.Audit,
		Now:   s.now,
		Cfg: app.AuthLifecycleConfig{
			BootstrapTokenTTLSeconds: s.authCfg.BootstrapTokenTTLSeconds,
			GrantIdleTimeoutMinutes:  s.authCfg.GrantIdleTimeoutMinutes,
			GrantAbsoluteMaxMinutes:  s.authCfg.GrantAbsoluteMaxMinutes,
			SessionTTLMinutes:        s.authCfg.SessionTTLMinutes,
		},
		NewBootstrapToken: func() string { return mustSecureToken("boot_") },
		NewGrantID:        func() string { return mustSecureToken("grant_") },
		NewSessionToken:   func() string { return mustSecureToken("sess_") },
		Persist:           s.persistAuthStore,
		AuditFailureHandler: func(err error) error {
			return s.closeDurabilityGate("audit", err)
		},
	}
}
