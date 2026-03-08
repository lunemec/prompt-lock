package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/core/ports"
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
	var ok bool
	r, ok = s.requireOperator(w, r)
	if !ok {
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
	if req.AgentID == "" || req.ContainerID == "" {
		http.Error(w, "agent_id and container_id are required", 400)
		return
	}
	t := auth.BootstrapToken{Token: mustSecureToken("boot_"), AgentID: req.AgentID, ContainerID: req.ContainerID, CreatedAt: s.now(), ExpiresAt: s.now().Add(time.Duration(s.authCfg.BootstrapTokenTTLSeconds) * time.Second)}
	s.authStore.SaveBootstrap(t)
	at, aid := actorFromRequest(r)
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "auth_bootstrap_created", Timestamp: s.now(), ActorType: at, ActorID: aid, AgentID: req.AgentID, Metadata: map[string]string{"container_id": req.ContainerID}})
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
	if req.Token == "" || req.ContainerID == "" {
		http.Error(w, "token and container_id are required", 400)
		return
	}
	bt, err := s.authStore.ConsumeBootstrap(req.Token, req.ContainerID, s.now())
	if err != nil {
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "auth_pair_denied", Timestamp: s.now(), ActorType: "agent", ActorID: "unknown", Metadata: map[string]string{"reason": err.Error(), "container_id": req.ContainerID}})
		http.Error(w, err.Error(), 403)
		return
	}
	g := auth.PairingGrant{GrantID: mustSecureToken("grant_"), AgentID: bt.AgentID, ContainerID: req.ContainerID, CreatedAt: s.now(), LastUsedAt: s.now(), IdleExpiresAt: s.now().Add(time.Duration(s.authCfg.GrantIdleTimeoutMinutes) * time.Minute), AbsoluteExpiresAt: s.now().Add(time.Duration(s.authCfg.GrantAbsoluteMaxMinutes) * time.Minute)}
	s.authStore.SaveGrant(g)
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "auth_pair_completed", Timestamp: s.now(), ActorType: "agent", ActorID: bt.AgentID, AgentID: bt.AgentID, Metadata: map[string]string{"container_id": req.ContainerID, "grant_id": g.GrantID}})
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
	st := auth.SessionToken{Token: mustSecureToken("sess_"), GrantID: g.GrantID, AgentID: g.AgentID, CreatedAt: now, ExpiresAt: now.Add(time.Duration(s.authCfg.SessionTTLMinutes) * time.Minute)}
	s.authStore.SaveSession(st)
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "auth_session_minted", Timestamp: s.now(), ActorType: "agent", ActorID: g.AgentID, AgentID: g.AgentID, Metadata: map[string]string{"grant_id": g.GrantID}})
	writeJSON(w, map[string]any{"session_token": st.Token, "expires_at": st.ExpiresAt})
}

func (s *server) handleAuthRevoke(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
	if !ok {
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
	at, aid := actorFromRequest(r)
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "auth_revoked", Timestamp: s.now(), ActorType: at, ActorID: aid, Metadata: map[string]string{"grant_id": req.GrantID, "session_id": req.SessionID}})
	writeJSON(w, map[string]any{"status": "revoked"})
}
