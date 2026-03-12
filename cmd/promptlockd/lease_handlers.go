package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func (s *server) handleRequestStatus(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		writeMappedError(w, ErrBadRequest, "request_id required")
		return
	}
	req, err := s.svc.Requests.GetRequest(requestID)
	if err != nil {
		kind, msg := stateStoreReadError(err)
		writeMappedError(w, kind, msg)
		return
	}
	writeJSON(w, map[string]any{"request_id": req.ID, "status": req.Status})
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireDurabilityReady(w) {
		return
	}
	var req struct {
		AgentID            string   `json:"agent_id"`
		TaskID             string   `json:"task_id"`
		Reason             string   `json:"reason"`
		TTLMinutes         int      `json:"ttl_minutes"`
		Secrets            []string `json:"secrets"`
		CommandFingerprint string   `json:"command_fingerprint"`
		WorkdirFingerprint string   `json:"workdir_fingerprint"`
		EnvPath            string   `json:"env_path"`
		EnvPathCanonical   string   `json:"env_path_canonical"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	if req.TTLMinutes == 0 {
		req.TTLMinutes = s.svc.Policy.DefaultTTLMinutes
	}
	req.CommandFingerprint = strings.TrimSpace(req.CommandFingerprint)
	if req.CommandFingerprint == "" {
		writeMappedError(w, ErrBadRequest, "command_fingerprint required")
		return
	}
	req.WorkdirFingerprint = strings.TrimSpace(req.WorkdirFingerprint)
	if req.WorkdirFingerprint == "" {
		writeMappedError(w, ErrBadRequest, "workdir_fingerprint required")
		return
	}
	req.EnvPath = strings.TrimSpace(req.EnvPath)
	req.EnvPathCanonical = strings.TrimSpace(req.EnvPathCanonical)
	if req.EnvPath != "" {
		envPathStore, err := s.ensureEnvPathSecretStore()
		if err != nil {
			writeMappedError(w, ErrServiceUnavailable, err.Error())
			return
		}
		canonicalPath, err := envPathStore.Canonicalize(req.EnvPath)
		if err != nil {
			writeMappedError(w, ErrBadRequest, err.Error())
			return
		}
		req.EnvPathCanonical = canonicalPath
	}
	result, err := s.svc.RequestLeaseWithPolicy(req.AgentID, req.TaskID, req.Reason, req.TTLMinutes, req.Secrets, req.CommandFingerprint, req.WorkdirFingerprint, req.EnvPath, req.EnvPathCanonical)
	if err != nil {
		var throttleErr *app.RequestThrottleError
		if errors.As(err, &throttleErr) {
			w.Header().Set("Retry-After", strconv.Itoa(throttleErr.RetryAfterSeconds()))
			writeMappedError(w, ErrRateLimited, throttleErr.Error())
			return
		}
		kind, msg := stateStoreMutationError(err)
		writeMappedError(w, kind, msg)
		return
	}
	if result.Reused {
		writeJSON(w, map[string]any{"request_id": result.Lease.RequestID, "status": "reused", "lease_token": result.Lease.Token, "expires_at": result.Lease.ExpiresAt})
		return
	}
	if err := s.persistRequestLeaseState(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return
	}
	writeJSON(w, map[string]any{"request_id": result.Request.ID, "status": result.Request.Status})
}

func (s *server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireDurabilityReady(w) {
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		writeMappedError(w, ErrBadRequest, "request_id required")
		return
	}
	var req approveReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	lease, err := s.svc.ApproveRequest(requestID, req.TTLMinutes)
	if err != nil {
		kind, msg := stateStoreMutationError(err)
		writeMappedError(w, kind, msg)
		return
	}
	if err := s.persistRequestLeaseState(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return
	}
	at, aid := actorFromRequest(r)
	if approvedReq, readErr := s.svc.Requests.GetRequest(requestID); readErr == nil {
		if strings.TrimSpace(approvedReq.EnvPath) != "" {
			s.svc.AuditEnvPathConfirmed(approvedReq.AgentID, approvedReq.TaskID, approvedReq.ID, approvedReq.EnvPath, approvedReq.EnvPathCanonical)
		}
	}
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "operator_approved_request", Timestamp: s.now(), ActorType: at, ActorID: aid, RequestID: requestID, LeaseToken: lease.Token})
	writeJSON(w, map[string]any{"status": "approved", "lease_token": lease.Token, "expires_at": lease.ExpiresAt})
}

func (s *server) handleAccess(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authCfg.AllowPlaintextSecretReturn {
		at, aid := actorFromRequest(r)
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "plaintext_secret_access_blocked", Timestamp: s.now(), ActorType: at, ActorID: aid})
		writeMappedError(w, ErrForbidden, "plaintext secret return disabled by policy")
		return
	}
	var req accessReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	v, err := s.svc.AccessSecret(req.LeaseToken, req.Secret, req.CommandFingerprint, req.WorkdirFingerprint)
	if err != nil {
		kind, msg := stateStoreAccessError(err)
		writeMappedError(w, kind, msg)
		return
	}
	writeJSON(w, map[string]any{"secret": req.Secret, "value": v})
}
