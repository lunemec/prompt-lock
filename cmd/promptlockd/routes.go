package main

import "net/http"

func (s *server) registerRoutes() {
	s.registerRoutesTo(http.DefaultServeMux)
}

func (s *server) registerRoutesTo(mux *http.ServeMux) {
	s.registerMetaRoutes(mux)
	s.registerLeaseRoutes(mux)
	s.registerAuthRoutes(mux)
	s.registerHostOpsRoutes(mux)
}

func (s *server) registerMetaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/meta/capabilities", s.handleMetaCapabilities)
	mux.HandleFunc("/v1/intents/resolve", s.handleResolveIntent)
}

func (s *server) registerLeaseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/requests/status", s.handleRequestStatus)
	mux.HandleFunc("/v1/requests/pending", s.handlePendingRequests)
	mux.HandleFunc("/v1/leases/request", s.handleRequest)
	mux.HandleFunc("/v1/leases/approve", s.handleApprove)
	mux.HandleFunc("/v1/leases/deny", s.handleDeny)
	mux.HandleFunc("/v1/leases/by-request", s.handleLeaseByRequest)
	mux.HandleFunc("/v1/leases/access", s.handleAccess)
	mux.HandleFunc("/v1/leases/execute", s.handleExecute)
}

func (s *server) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/auth/bootstrap/create", s.handleAuthBootstrapCreate)
	mux.HandleFunc("/v1/auth/pair/complete", s.handleAuthPairComplete)
	mux.HandleFunc("/v1/auth/session/mint", s.handleAuthSessionMint)
	mux.HandleFunc("/v1/auth/revoke", s.handleAuthRevoke)
}

func (s *server) registerHostOpsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/host/docker/execute", s.handleHostDockerExecute)
}
