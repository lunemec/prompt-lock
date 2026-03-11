package main

import (
	"encoding/json"
	"net/http"

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
		writeMappedError(w, ErrNotFound, err.Error())
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
	var req leaseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	if req.TTLMinutes == 0 {
		req.TTLMinutes = s.svc.Policy.DefaultTTLMinutes
	}
	result, err := s.svc.RequestLeaseWithPolicy(req.AgentID, req.TaskID, req.Reason, req.TTLMinutes, req.Secrets, req.CommandFingerprint, req.WorkdirFingerprint)
	if err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
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
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	if err := s.persistRequestLeaseState(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return
	}
	at, aid := actorFromRequest(r)
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
		writeMappedError(w, ErrForbidden, err.Error())
		return
	}
	writeJSON(w, map[string]any{"secret": req.Secret, "value": v})
}
