package main

import "net/http"

func (s *server) handleLeaseByRequest(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		http.Error(w, "request_id required", 400)
		return
	}
	lease, err := s.svc.Leases.GetLeaseByRequestID(requestID)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, map[string]any{"lease_token": lease.Token, "expires_at": lease.ExpiresAt})
}
