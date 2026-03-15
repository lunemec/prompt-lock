package main

import (
	"net/http"
	"strings"
)

type denyReq struct {
	Reason string `json:"reason"`
}

func (s *server) handleDeny(w http.ResponseWriter, r *http.Request) {
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
	var req denyReq
	if err := decodeOptionalJSONBody(r, &req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	at, aid := actorFromRequest(r)
	denied, err := s.svc.DenyRequestWithActor(requestID, req.Reason, at, aid)
	if err != nil {
		kind, msg := stateStoreMutationError(err)
		writeMappedError(w, kind, msg)
		return
	}
	if strings.TrimSpace(denied.EnvPath) != "" {
		_ = s.svc.AuditEnvPathRejected(denied.AgentID, denied.TaskID, denied.ID, denied.EnvPath, denied.EnvPathCanonical, req.Reason)
	}
	writeJSON(w, map[string]any{"request_id": denied.ID, "status": denied.Status})
}
