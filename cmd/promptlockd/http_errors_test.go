package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteMappedErrorStatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"bad_request", ErrBadRequest, http.StatusBadRequest},
		{"unauthorized", ErrUnauthorized, http.StatusUnauthorized},
		{"forbidden", ErrForbidden, http.StatusForbidden},
		{"not_found", ErrNotFound, http.StatusNotFound},
		{"internal", ErrInternal, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeMappedError(rr, tc.err, "x")
			if rr.Code != tc.status {
				t.Fatalf("expected %d got %d", tc.status, rr.Code)
			}
		})
	}
}
