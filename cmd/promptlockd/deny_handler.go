package main

import (
	"net/http"
	"strings"

	"github.com/lunemec/promptlock/internal/core/ports"
)

type denyReq struct {
	Reason string `json:"reason"`
}

func (s *server) handleDeny(w http.ResponseWriter, r *http.Request) {
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
	var req denyReq
	if err := decodeOptionalJSONBody(r, &req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	denied, err := s.svc.DenyRequest(requestID, req.Reason)
	if err != nil {
		kind, msg := stateStoreMutationError(err)
		writeMappedError(w, kind, msg)
		return
	}
	if err := s.persistRequestLeaseState(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return
	}
	if strings.TrimSpace(denied.EnvPath) != "" {
		s.svc.AuditEnvPathRejected(denied.AgentID, denied.TaskID, denied.ID, denied.EnvPath, denied.EnvPathCanonical, req.Reason)
	}
	at, aid := actorFromRequest(r)
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "operator_denied_request", Timestamp: s.now(), ActorType: at, ActorID: aid, RequestID: requestID, Metadata: map[string]string{"reason": req.Reason}})
	writeJSON(w, map[string]any{"request_id": denied.ID, "status": denied.Status})
}
