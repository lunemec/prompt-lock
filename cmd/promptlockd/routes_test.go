package main

import (
	"net/http"
	"testing"
)

func TestRegisterAgentRoutesToExposesOnlyAgentEndpoints(t *testing.T) {
	s := &server{}
	mux := http.NewServeMux()
	s.registerAgentRoutesTo(mux)

	requireRegisteredPaths(t, mux, []string{
		"/v1/meta/capabilities",
		"/v1/intents/resolve",
		"/v1/requests/status",
		"/v1/leases/request",
		"/v1/leases/cancel",
		"/v1/leases/by-request",
		"/v1/leases/access",
		"/v1/leases/execute",
		"/v1/auth/pair/complete",
		"/v1/auth/session/mint",
	})
	requireMissingPaths(t, mux, []string{
		"/v1/requests/pending",
		"/v1/leases/approve",
		"/v1/leases/deny",
		"/v1/auth/bootstrap/create",
		"/v1/auth/revoke",
		"/v1/host/docker/execute",
	})
}

func TestRegisterOperatorRoutesToExposesOnlyOperatorEndpoints(t *testing.T) {
	s := &server{}
	mux := http.NewServeMux()
	s.registerOperatorRoutesTo(mux)

	requireRegisteredPaths(t, mux, []string{
		"/v1/meta/capabilities",
		"/v1/requests/pending",
		"/v1/leases/approve",
		"/v1/leases/deny",
		"/v1/auth/bootstrap/create",
		"/v1/auth/revoke",
		"/v1/host/docker/execute",
	})
	requireMissingPaths(t, mux, []string{
		"/v1/intents/resolve",
		"/v1/requests/status",
		"/v1/leases/request",
		"/v1/leases/cancel",
		"/v1/leases/by-request",
		"/v1/leases/access",
		"/v1/leases/execute",
		"/v1/auth/pair/complete",
		"/v1/auth/session/mint",
	})
}

func TestRegisterLegacyRoutesToExposesFullRouteSet(t *testing.T) {
	s := &server{}
	mux := http.NewServeMux()
	s.registerLegacyRoutesTo(mux)

	requireRegisteredPaths(t, mux, []string{
		"/v1/meta/capabilities",
		"/v1/intents/resolve",
		"/v1/requests/status",
		"/v1/requests/pending",
		"/v1/leases/request",
		"/v1/leases/approve",
		"/v1/leases/deny",
		"/v1/leases/cancel",
		"/v1/leases/by-request",
		"/v1/leases/access",
		"/v1/leases/execute",
		"/v1/auth/bootstrap/create",
		"/v1/auth/pair/complete",
		"/v1/auth/session/mint",
		"/v1/auth/revoke",
		"/v1/host/docker/execute",
	})
}

func requireRegisteredPaths(t *testing.T, mux *http.ServeMux, paths []string) {
	t.Helper()
	for _, p := range paths {
		r, _ := http.NewRequest(http.MethodGet, p, nil)
		_, pattern := mux.Handler(r)
		if pattern == "" {
			t.Fatalf("expected route %s to be registered", p)
		}
	}
}

func requireMissingPaths(t *testing.T, mux *http.ServeMux, paths []string) {
	t.Helper()
	for _, p := range paths {
		r, _ := http.NewRequest(http.MethodGet, p, nil)
		_, pattern := mux.Handler(r)
		if pattern != "" {
			t.Fatalf("expected route %s to be absent, got pattern %q", p, pattern)
		}
	}
}
