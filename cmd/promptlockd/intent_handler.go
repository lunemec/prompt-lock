package main

import (
	"encoding/json"
	"net/http"
)

func (s *server) handleResolveIntent(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	var req resolveIntentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	secrets, ok := s.intents[req.Intent]
	if !ok || len(secrets) == 0 {
		writeMappedError(w, ErrNotFound, "unknown intent")
		return
	}
	writeJSON(w, map[string]any{"intent": req.Intent, "secrets": secrets})
}
