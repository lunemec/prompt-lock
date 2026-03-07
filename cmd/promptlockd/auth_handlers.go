package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/lunemec/promptlock/internal/auth"
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
	GrantID   string `json:"grant_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

func (s *server) handleAuthBootstrapCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.authEnabled {
		http.Error(w, "auth disabled", 400)
		return
	}
	var req bootstrapCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t := auth.BootstrapToken{Token: "boot_" + itoa(s.nextSeq()), AgentID: req.AgentID, CreatedAt: s.now(), ExpiresAt: s.now().Add(time.Duration(s.authCfg.BootstrapTokenTTLSeconds) * time.Second)}
	s.authStore.SaveBootstrap(t)
	writeJSON(w, map[string]any{"bootstrap_token": t.Token, "expires_at": t.ExpiresAt})
}

func (s *server) handleAuthPairComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.authEnabled {
		http.Error(w, "auth disabled", 400)
		return
	}
	var req bootstrapConsumeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	bt, err := s.authStore.ConsumeBootstrap(req.Token, s.now())
	if err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	g := auth.PairingGrant{GrantID: "grant_" + itoa(s.nextSeq()), AgentID: bt.AgentID, ContainerID: req.ContainerID, CreatedAt: s.now(), LastUsedAt: s.now(), IdleExpiresAt: s.now().Add(time.Duration(s.authCfg.GrantIdleTimeoutMinutes) * time.Minute), AbsoluteExpiresAt: s.now().Add(time.Duration(s.authCfg.GrantAbsoluteMaxMinutes) * time.Minute)}
	s.authStore.SaveGrant(g)
	writeJSON(w, map[string]any{"grant_id": g.GrantID, "idle_expires_at": g.IdleExpiresAt, "absolute_expires_at": g.AbsoluteExpiresAt})
}

func (s *server) handleAuthSessionMint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.authEnabled {
		http.Error(w, "auth disabled", 400)
		return
	}
	var req mintSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	g, err := s.authStore.GetGrant(req.GrantID)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	now := s.now()
	if g.Revoked || !now.Before(g.IdleExpiresAt) || !now.Before(g.AbsoluteExpiresAt) {
		http.Error(w, "grant expired or revoked", 403)
		return
	}
	g.LastUsedAt = now
	g.IdleExpiresAt = now.Add(time.Duration(s.authCfg.GrantIdleTimeoutMinutes) * time.Minute)
	s.authStore.UpdateGrant(g)
	st := auth.SessionToken{Token: "sess_" + itoa(s.nextSeq()), GrantID: g.GrantID, AgentID: g.AgentID, CreatedAt: now, ExpiresAt: now.Add(time.Duration(s.authCfg.SessionTTLMinutes) * time.Minute)}
	s.authStore.SaveSession(st)
	writeJSON(w, map[string]any{"session_token": st.Token, "expires_at": st.ExpiresAt})
}

func (s *server) handleAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.authEnabled {
		http.Error(w, "auth disabled", 400)
		return
	}
	var req revokeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.GrantID != "" {
		if err := s.authStore.RevokeGrant(req.GrantID); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
	}
	if req.SessionID != "" {
		if err := s.authStore.RevokeSession(req.SessionID); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
	}
	writeJSON(w, map[string]any{"status": "revoked"})
}
