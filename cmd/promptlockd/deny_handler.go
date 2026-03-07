package main

import (
	"encoding/json"
	"net/http"

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
		http.Error(w, "method not allowed", 405)
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		http.Error(w, "request_id required", 400)
		return
	}
	var req denyReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	denied, err := s.svc.DenyRequest(requestID, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	at, aid := actorFromRequest(r)
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "operator_denied_request", Timestamp: s.now(), ActorType: at, ActorID: aid, RequestID: requestID, Metadata: map[string]string{"reason": req.Reason}})
	writeJSON(w, map[string]any{"request_id": denied.ID, "status": denied.Status})
}
