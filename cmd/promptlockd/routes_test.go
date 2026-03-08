package main

import (
	"net/http"
	"testing"
)

func TestRegisterRoutesToRegistersEndpoints(t *testing.T) {
	s := &server{}
	mux := http.NewServeMux()
	s.registerRoutesTo(mux)

	paths := []string{
		"/v1/meta/capabilities",
		"/v1/intents/resolve",
		"/v1/requests/status",
		"/v1/requests/pending",
		"/v1/leases/request",
		"/v1/leases/approve",
		"/v1/leases/deny",
		"/v1/leases/by-request",
		"/v1/leases/access",
		"/v1/leases/execute",
		"/v1/auth/bootstrap/create",
		"/v1/auth/pair/complete",
		"/v1/auth/session/mint",
		"/v1/auth/revoke",
		"/v1/host/docker/execute",
	}

	for _, p := range paths {
		r, _ := http.NewRequest(http.MethodGet, p, nil)
		_, pattern := mux.Handler(r)
		if pattern == "" {
			t.Fatalf("expected route %s to be registered", p)
		}
	}
}
