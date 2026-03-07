package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/audit"
	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type server struct {
	svc         app.Service
	intents     map[string][]string
	authEnabled bool
	authCfg     config.AuthConfig
	authStore   *auth.Store
	seq         *uint64
	now         func() time.Time
}

type leaseReq struct {
	AgentID            string   `json:"agent_id"`
	TaskID             string   `json:"task_id"`
	Reason             string   `json:"reason"`
	TTLMinutes         int      `json:"ttl_minutes"`
	Secrets            []string `json:"secrets"`
	CommandFingerprint string   `json:"command_fingerprint"`
	WorkdirFingerprint string   `json:"workdir_fingerprint"`
}

type approveReq struct {
	TTLMinutes int `json:"ttl_minutes"`
}
type accessReq struct {
	LeaseToken         string `json:"lease_token"`
	Secret             string `json:"secret"`
	CommandFingerprint string `json:"command_fingerprint"`
	WorkdirFingerprint string `json:"workdir_fingerprint"`
}

type resolveIntentReq struct {
	Intent string `json:"intent"`
}

func main() {
	cfgPath := getenv("PROMPTLOCK_CONFIG", "/etc/promptlock/config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	if v := os.Getenv("PROMPTLOCK_AUDIT_PATH"); v != "" {
		cfg.AuditPath = v
	}
	if v := os.Getenv("PROMPTLOCK_ADDR"); v != "" {
		cfg.Address = v
	}
	if v := os.Getenv("PROMPTLOCK_UNIX_SOCKET"); v != "" {
		cfg.UnixSocket = v
	}
	if v := os.Getenv("PROMPTLOCK_OPERATOR_TOKEN"); v != "" {
		cfg.Auth.OperatorToken = v
	}

	store := memory.NewStore()
	store.SetSecret("github_token", getenv("PROMPTLOCK_DEMO_GITHUB_TOKEN", "DEMO_GITHUB_TOKEN"))
	store.SetSecret("npm_token", getenv("PROMPTLOCK_DEMO_NPM_TOKEN", "DEMO_NPM_TOKEN"))
	for _, s := range cfg.Secrets {
		if s.Name != "" {
			store.SetSecret(s.Name, s.Value)
		}
	}

	sink, err := audit.NewFileSink(cfg.AuditPath)
	if err != nil {
		log.Fatal(err)
	}
	defer sink.Close()

	var seq uint64
	newReq := func() string { return "req_" + itoa(atomic.AddUint64(&seq, 1)) }
	newLease := func() string { return "lease_" + itoa(atomic.AddUint64(&seq, 1)) }

	svc := app.Service{
		Policy:       cfg.ToPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        sink,
		Now:          func() time.Time { return time.Now().UTC() },
		NewRequestID: newReq,
		NewLeaseTok:  newLease,
	}

	if cfg.Auth.EnableAuth && cfg.Auth.OperatorToken == "" {
		log.Fatal("auth enabled but operator_token is empty")
	}
	if cfg.Auth.EnableAuth && cfg.UnixSocket == "" && !isLocalAddress(cfg.Address) && getenv("PROMPTLOCK_ALLOW_INSECURE_TCP", "") != "1" {
		log.Fatal("auth enabled on non-local TCP without unix socket; set unix_socket or PROMPTLOCK_ALLOW_INSECURE_TCP=1")
	}
	authStore := auth.NewStore()
	s := &server{svc: svc, intents: cfg.Intents, authEnabled: cfg.Auth.EnableAuth, authCfg: cfg.Auth, authStore: authStore, seq: &seq, now: func() time.Time { return time.Now().UTC() }}
	http.HandleFunc("/v1/intents/resolve", s.handleResolveIntent)
	http.HandleFunc("/v1/requests/status", s.handleRequestStatus)
	http.HandleFunc("/v1/requests/pending", s.handlePendingRequests)
	http.HandleFunc("/v1/leases/request", s.handleRequest)
	http.HandleFunc("/v1/leases/approve", s.handleApprove)
	http.HandleFunc("/v1/leases/deny", s.handleDeny)
	http.HandleFunc("/v1/leases/by-request", s.handleLeaseByRequest)
	http.HandleFunc("/v1/leases/access", s.handleAccess)
	http.HandleFunc("/v1/auth/bootstrap/create", s.handleAuthBootstrapCreate)
	http.HandleFunc("/v1/auth/pair/complete", s.handleAuthPairComplete)
	http.HandleFunc("/v1/auth/session/mint", s.handleAuthSessionMint)
	http.HandleFunc("/v1/auth/revoke", s.handleAuthRevoke)

	if cfg.UnixSocket != "" {
		_ = os.Remove(cfg.UnixSocket)
		ln, err := net.Listen("unix", cfg.UnixSocket)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.Chmod(cfg.UnixSocket, 0o600); err != nil {
			log.Fatal(err)
		}
		defer func() { _ = os.Remove(cfg.UnixSocket) }()
		log.Printf("promptlock listening on unix socket %s", cfg.UnixSocket)
		log.Fatal(http.Serve(ln, nil))
	}

	log.Printf("promptlock listening on %s", cfg.Address)
	log.Fatal(http.ListenAndServe(cfg.Address, nil))
}

func (s *server) handleResolveIntent(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req resolveIntentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	secrets, ok := s.intents[req.Intent]
	if !ok || len(secrets) == 0 {
		http.Error(w, "unknown intent", 404)
		return
	}
	writeJSON(w, map[string]any{"intent": req.Intent, "secrets": secrets})
}

func (s *server) handleRequestStatus(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		http.Error(w, "request_id required", 400)
		return
	}
	req, err := s.svc.Requests.GetRequest(requestID)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, map[string]any{"request_id": req.ID, "status": req.Status})
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req leaseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.TTLMinutes == 0 {
		req.TTLMinutes = s.svc.Policy.DefaultTTLMinutes
	}
	created, err := s.svc.RequestLease(req.AgentID, req.TaskID, req.Reason, req.TTLMinutes, req.Secrets, req.CommandFingerprint, req.WorkdirFingerprint)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"request_id": created.ID, "status": created.Status})
}

func (s *server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		http.Error(w, "request_id required", 400)
		return
	}
	var req approveReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	lease, err := s.svc.ApproveRequest(requestID, req.TTLMinutes)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	at, aid := actorFromRequest(r)
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "operator_approved_request", Timestamp: s.now(), ActorType: at, ActorID: aid, RequestID: requestID, LeaseToken: lease.Token})
	writeJSON(w, map[string]any{"status": "approved", "lease_token": lease.Token, "expires_at": lease.ExpiresAt})
}

func (s *server) handleAccess(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req accessReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	v, err := s.svc.AccessSecret(req.LeaseToken, req.Secret, req.CommandFingerprint, req.WorkdirFingerprint)
	if err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	writeJSON(w, map[string]any{"secret": req.Secret, "value": v})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func (s *server) nextSeq() uint64 { return atomic.AddUint64(s.seq, 1) }

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }

func isLocalAddress(addr string) bool {
	a := strings.TrimSpace(strings.ToLower(addr))
	if a == "" {
		return false
	}
	if strings.HasPrefix(a, "127.0.0.1:") || strings.HasPrefix(a, "localhost:") {
		return true
	}
	if a == "127.0.0.1" || a == "localhost" {
		return true
	}
	return false
}
