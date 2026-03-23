package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/lunemec/promptlock/internal/app"
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
	if !s.requireDurabilityReady(w) {
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		writeMappedError(w, ErrBadRequest, "request_id required")
		return
	}
	_, actorID := actorFromRequest(r)
	if !s.authEnabled {
		actorID = ""
	}
	req, err := s.svc.RequestStatusByAgent(requestID, actorID)
	if err != nil {
		kind, msg := stateStoreReadError(err)
		writeMappedError(w, kind, msg)
		return
	}
	writeJSON(w, map[string]any{"request_id": req.ID, "status": req.Status})
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.ensureRequestLeaseStateCommitter()
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
		Intent             string   `json:"intent"`
		Reason             string   `json:"reason"`
		TTLMinutes         int      `json:"ttl_minutes"`
		Secrets            []string `json:"secrets"`
		CommandFingerprint string   `json:"command_fingerprint"`
		WorkdirFingerprint string   `json:"workdir_fingerprint"`
		CommandSummary     string   `json:"command_summary"`
		WorkdirSummary     string   `json:"workdir_summary"`
		EnvPath            string   `json:"env_path"`
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
	effectiveAgentID := strings.TrimSpace(req.AgentID)
	if s.authEnabled {
		_, sessionAgentID := actorFromRequest(r)
		if effectiveAgentID != "" && effectiveAgentID != sessionAgentID {
			writeMappedError(w, ErrForbidden, "agent_id does not match authenticated session")
			return
		}
		effectiveAgentID = sessionAgentID
	}
	result, err := s.svc.RequestLeaseWithPolicyAndIntentAndSummary(effectiveAgentID, req.TaskID, req.Reason, req.TTLMinutes, req.Secrets, req.Intent, req.CommandFingerprint, req.WorkdirFingerprint, req.EnvPath, req.CommandSummary, req.WorkdirSummary)
	if err != nil {
		var throttleErr *app.RequestThrottleError
		if errors.As(err, &throttleErr) {
			w.Header().Set("Retry-After", strconv.Itoa(throttleErr.RetryAfterSeconds()))
			writeMappedError(w, ErrRateLimited, throttleErr.Error())
			return
		}
		if errors.Is(err, app.ErrSecretBackendUnavailable) {
			writeMappedError(w, ErrServiceUnavailable, secretBackendUnavailableMessage)
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
	writeJSON(w, map[string]any{"request_id": result.Request.ID, "status": result.Request.Status})
}

func (s *server) handleApprove(w http.ResponseWriter, r *http.Request) {
	s.ensureRequestLeaseStateCommitter()
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
	if err := decodeOptionalJSONBody(r, &req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	at, aid := actorFromRequest(r)
	lease, err := s.svc.ApproveRequestWithActor(requestID, req.TTLMinutes, at, aid)
	if err != nil {
		kind, msg := stateStoreMutationError(err)
		writeMappedError(w, kind, msg)
		return
	}
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
	if !s.requireDurabilityReady(w) {
		return
	}
	at, aid := actorFromRequest(r)
	if err := s.svc.RejectPlaintextSecretAccess(at, aid); err != nil {
		if errors.Is(err, app.ErrPlaintextSecretReturnDisabled) {
			writeMappedError(w, ErrForbidden, err.Error())
			return
		}
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return
	}
	var req accessReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	_, actorID := actorFromRequest(r)
	if !s.authEnabled {
		actorID = ""
	}
	v, err := s.svc.AccessSecretByAgent(actorID, req.LeaseToken, req.Secret, req.CommandFingerprint, req.WorkdirFingerprint)
	if err != nil {
		kind, msg := stateStoreAccessError(err)
		writeMappedError(w, kind, msg)
		return
	}
	writeJSON(w, map[string]any{"secret": req.Secret, "value": v})
}
