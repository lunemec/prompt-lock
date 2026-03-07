package main

import "net/http"

func (s *server) handlePendingRequests(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	items, err := s.svc.Requests.ListPendingRequests()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"pending": items})
}
