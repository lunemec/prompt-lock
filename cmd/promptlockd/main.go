package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/audit"
	"github.com/lunemec/promptlock/internal/adapters/envsecret"
	"github.com/lunemec/promptlock/internal/adapters/filesecret"
	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type server struct {
	svc                  app.Service
	intents              map[string][]string
	authEnabled          bool
	authCfg              config.AuthConfig
	execPolicy           config.ExecutionPolicy
	hostOpsPolicy        config.HostOpsPolicy
	networkEgressPolicy  config.NetworkEgressPolicy
	securityProfile      string
	authStore            *auth.Store
	authStoreFile        string
	authLimiter          *authRateLimiter
	policyEngine         app.ControlPlanePolicy
	unixSocketConfigured bool
	now                  func() time.Time
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
	secretStore := any(store).(ports.SecretStore)
	sourceType := strings.ToLower(strings.TrimSpace(cfg.SecretSource.Type))
	switch sourceType {
	case "env":
		secretStore = envsecret.New(cfg.SecretSource.EnvPrefix)
	case "file":
		fs, err := filesecret.New(cfg.SecretSource.FilePath)
		if err != nil {
			log.Fatalf("init file secret source: %v", err)
		}
		secretStore = fs
	default:
		store.SetSecret("github_token", getenv("PROMPTLOCK_DEMO_GITHUB_TOKEN", "DEMO_GITHUB_TOKEN"))
		store.SetSecret("npm_token", getenv("PROMPTLOCK_DEMO_NPM_TOKEN", "DEMO_NPM_TOKEN"))
		for _, s := range cfg.Secrets {
			if s.Name != "" {
				store.SetSecret(s.Name, s.Value)
			}
		}
	}

	sink, err := audit.NewFileSink(cfg.AuditPath)
	if err != nil {
		log.Fatal(err)
	}
	defer sink.Close()

	newReq := func() string { return mustSecureToken("req_") }
	newLease := func() string { return mustSecureToken("lease_") }

	svc := app.Service{
		Policy:       cfg.ToPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      secretStore,
		Audit:        sink,
		Now:          func() time.Time { return time.Now().UTC() },
		NewRequestID: newReq,
		NewLeaseTok:  newLease,
	}

	if err := validateSecurityProfile(cfg, getenv("PROMPTLOCK_ALLOW_INSECURE_PROFILE", "")); err != nil {
		log.Fatal(err)
	}
	if cfg.Auth.EnableAuth && cfg.Auth.OperatorToken == "" {
		log.Fatal("auth enabled but operator_token is empty")
	}
	if err := validateSecretSourceSafety(cfg); err != nil {
		log.Fatal(err)
	}
	allowInsecureTCP := getenv("PROMPTLOCK_ALLOW_INSECURE_TCP", "")
	if err := validateTransportSafety(cfg, allowInsecureTCP); err != nil {
		log.Fatal(err)
	}
	insecureTCPOverride := cfg.Auth.EnableAuth && cfg.UnixSocket == "" && !cfg.TLS.Enable && !isLocalAddress(cfg.Address) && allowInsecureTCP == "1"
	if insecureTCPOverride {
		log.Printf("WARNING: insecure TCP override enabled (PROMPTLOCK_ALLOW_INSECURE_TCP=1) on %s", cfg.Address)
	}
	authStore := auth.NewStore()
	if cfg.Auth.EnableAuth && strings.TrimSpace(cfg.Auth.StoreFile) != "" {
		if err := authStore.LoadFromFile(cfg.Auth.StoreFile); err != nil {
			log.Fatalf("load auth store: %v", err)
		}
	}
	policyEngine := app.NewDefaultControlPlanePolicy(cfg.ExecutionPolicy, cfg.HostOpsPolicy, cfg.NetworkEgressPolicy)
	s := &server{svc: svc, intents: cfg.Intents, authEnabled: cfg.Auth.EnableAuth, authCfg: cfg.Auth, execPolicy: cfg.ExecutionPolicy, hostOpsPolicy: cfg.HostOpsPolicy, networkEgressPolicy: cfg.NetworkEgressPolicy, securityProfile: strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)), authStore: authStore, authStoreFile: cfg.Auth.StoreFile, authLimiter: newAuthRateLimiter(cfg.Auth), policyEngine: policyEngine, unixSocketConfigured: cfg.UnixSocket != "", now: func() time.Time { return time.Now().UTC() }}
	if strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)) == "hardened" && strings.EqualFold(strings.TrimSpace(cfg.SecretSource.Type), "in_memory") {
		log.Printf("WARNING: hardened profile using in_memory secret source (set secret_source.type=env or external backend)")
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "startup_inmemory_secret_source_warning", Timestamp: s.now(), ActorType: "system", ActorID: "promptlockd"})
	}
	if insecureTCPOverride {
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "startup_insecure_tcp_override", Timestamp: s.now(), ActorType: "system", ActorID: "promptlockd", Metadata: map[string]string{"address": cfg.Address}})
	}
	if cfg.Auth.EnableAuth {
		startAuthCleanupLoop(s)
	}
	s.registerRoutes()

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

	if cfg.TLS.Enable {
		tlsCfg, err := buildTLSConfig(cfg)
		if err != nil {
			log.Fatal(err)
		}
		srv := &http.Server{Addr: cfg.Address, Handler: nil, TLSConfig: tlsCfg}
		log.Printf("promptlock listening with tls on %s", cfg.Address)
		log.Fatal(srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile))
	}

	log.Printf("promptlock listening on %s", cfg.Address)
	log.Fatal(http.ListenAndServe(cfg.Address, nil))
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

func validateSecretSourceSafety(cfg config.Config) error {
	src := strings.ToLower(strings.TrimSpace(cfg.SecretSource.Type))
	if src == "" {
		src = "in_memory"
	}
	if src != "in_memory" && src != "env" && src != "file" {
		return fmt.Errorf("unsupported secret_source.type %q (supported: in_memory, env, file)", cfg.SecretSource.Type)
	}
	if src == "file" && strings.TrimSpace(cfg.SecretSource.FilePath) == "" {
		return fmt.Errorf("secret_source.type=file requires secret_source.file_path")
	}
	if strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)) == "hardened" && src == "in_memory" {
		mode := strings.ToLower(strings.TrimSpace(cfg.SecretSource.InMemoryHardened))
		if mode == "fail" {
			return fmt.Errorf("hardened profile with in_memory secret_source is disallowed (secret_source.in_memory_hardened=fail)")
		}
	}
	return nil
}

func validateTransportSafety(cfg config.Config, allowInsecure string) error {
	if err := validateTLSConfig(cfg); err != nil {
		return err
	}
	if cfg.Auth.EnableAuth && cfg.UnixSocket == "" && !cfg.TLS.Enable && !isLocalAddress(cfg.Address) && allowInsecure != "1" {
		return fmt.Errorf("auth enabled on non-local TCP without unix socket or tls; set unix_socket, enable tls, or PROMPTLOCK_ALLOW_INSECURE_TCP=1")
	}
	return nil
}

func validateTLSConfig(cfg config.Config) error {
	if !cfg.TLS.Enable {
		return nil
	}
	if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
		return fmt.Errorf("tls enabled requires tls.cert_file and tls.key_file")
	}
	if cfg.TLS.RequireClientCert && strings.TrimSpace(cfg.TLS.ClientCAFile) == "" {
		return fmt.Errorf("tls.require_client_cert=true requires tls.client_ca_file")
	}
	return nil
}

func validateSecurityProfile(cfg config.Config, allowInsecureProfile string) error {
	profile := strings.TrimSpace(strings.ToLower(cfg.SecurityProfile))
	if profile == "" {
		profile = "dev"
	}
	if profile == "dev" {
		return nil
	}
	if profile == "insecure" && allowInsecureProfile != "1" {
		return fmt.Errorf("security_profile=insecure requires explicit opt-in: set PROMPTLOCK_ALLOW_INSECURE_PROFILE=1")
	}
	if profile != "dev" && !cfg.Auth.EnableAuth {
		return fmt.Errorf("security_profile=%s requires auth.enable_auth=true", profile)
	}
	return nil
}

func buildTLSConfig(cfg config.Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if !cfg.TLS.RequireClientCert {
		return tlsCfg, nil
	}
	caPath := strings.TrimSpace(cfg.TLS.ClientCAFile)
	caBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read tls client ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("parse tls client ca: no certificates found")
	}
	tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	tlsCfg.ClientCAs = pool
	return tlsCfg, nil
}
