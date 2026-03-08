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
	items, err := s.svc.Requests.ListPendingRequests()
	if err != nil {
		writeMappedError(w, ErrInternal, err.Error())
		return
	}
	writeJSON(w, map[string]any{"pending": items})
}
