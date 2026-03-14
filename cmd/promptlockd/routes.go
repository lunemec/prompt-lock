package main

import "net/http"

func (s *server) registerRoutes() {
	s.registerLegacyRoutesTo(http.DefaultServeMux)
}

func (s *server) registerLegacyRoutesTo(mux *http.ServeMux) {
	s.registerSharedRoutes(mux)
	s.registerAgentRoutesOnly(mux)
	s.registerOperatorRoutesOnly(mux)
}

func (s *server) registerAgentRoutesTo(mux *http.ServeMux) {
	s.registerSharedRoutes(mux)
	s.registerAgentRoutesOnly(mux)
}

func (s *server) registerOperatorRoutesTo(mux *http.ServeMux) {
	s.registerSharedRoutes(mux)
	s.registerOperatorRoutesOnly(mux)
}

func (s *server) registerSharedRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/meta/capabilities", s.handleMetaCapabilities)
}

func (s *server) registerAgentRoutesOnly(mux *http.ServeMux) {
	mux.HandleFunc("/v1/intents/resolve", s.handleResolveIntent)
	mux.HandleFunc("/v1/requests/status", s.handleRequestStatus)
	mux.HandleFunc("/v1/leases/request", s.handleRequest)
	mux.HandleFunc("/v1/leases/cancel", s.handleCancel)
	mux.HandleFunc("/v1/leases/by-request", s.handleLeaseByRequest)
	mux.HandleFunc("/v1/leases/access", s.handleAccess)
	mux.HandleFunc("/v1/leases/execute", s.handleExecute)
	mux.HandleFunc("/v1/auth/pair/complete", s.handleAuthPairComplete)
	mux.HandleFunc("/v1/auth/session/mint", s.handleAuthSessionMint)
}

func (s *server) registerOperatorRoutesOnly(mux *http.ServeMux) {
	mux.HandleFunc("/v1/requests/pending", s.handlePendingRequests)
	mux.HandleFunc("/v1/leases/approve", s.handleApprove)
	mux.HandleFunc("/v1/leases/deny", s.handleDeny)
	mux.HandleFunc("/v1/auth/bootstrap/create", s.handleAuthBootstrapCreate)
	mux.HandleFunc("/v1/auth/revoke", s.handleAuthRevoke)
	mux.HandleFunc("/v1/host/docker/execute", s.handleHostDockerExecute)
}
