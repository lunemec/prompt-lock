package main

import "net/http"

func (s *server) handlePendingRequests(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
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
	items, err := s.svc.ListPendingRequests()
	if err != nil {
		kind, msg := stateStoreListError(err)
		writeMappedError(w, kind, msg)
		return
	}
	writeJSON(w, map[string]any{"pending": items})
}
