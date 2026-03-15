package main

import (
	"net/http"
	"strings"
)

type cancelReq struct {
	Reason string `json:"reason"`
}

func (s *server) handleCancel(w http.ResponseWriter, r *http.Request) {
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
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		writeMappedError(w, ErrBadRequest, "request_id required")
		return
	}
	var req cancelReq
	if err := decodeOptionalJSONBody(r, &req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		req.Reason = "agent requested cancellation"
	}
	_, actorID := actorFromRequest(r)
	if !s.authEnabled {
		actorID = ""
	}
	cancelled, err := s.svc.CancelRequestByAgent(requestID, actorID, req.Reason)
	if err != nil {
		kind, msg := stateStoreCancelMutationError(err)
		writeMappedError(w, kind, msg)
		return
	}
	writeJSON(w, map[string]any{"request_id": cancelled.ID, "status": cancelled.Status})
}
