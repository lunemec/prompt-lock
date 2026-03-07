package main

import (
	"encoding/json"
	"net/http"
)

type denyReq struct {
	Reason string `json:"reason"`
}

func (s *server) handleDeny(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
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
	writeJSON(w, map[string]any{"request_id": denied.ID, "status": denied.Status})
}
