package main

import "net/http"

func (s *server) handleLeaseByRequest(w http.ResponseWriter, r *http.Request) {
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
	lease, err := s.svc.LeaseByRequestForAgent(requestID, actorID)
	if err != nil {
		kind, msg := stateStoreReadError(err)
		writeMappedError(w, kind, msg)
		return
	}
	writeJSON(w, map[string]any{"lease_token": lease.Token, "expires_at": lease.ExpiresAt})
}
