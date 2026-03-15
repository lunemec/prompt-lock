package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/audit"
	"github.com/lunemec/promptlock/internal/adapters/envpath"
	"github.com/lunemec/promptlock/internal/adapters/envsecret"
	"github.com/lunemec/promptlock/internal/adapters/externalsecret"
	"github.com/lunemec/promptlock/internal/adapters/externalstate"
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
	authStorePersister   authStorePersister
	authStoreFile        string
	authStoreKey         []byte
	stateStoreFile       string
	stateStorePersister  stateStorePersister
	authLimiter          *authRateLimiter
	policyEngine         app.ControlPlanePolicy
	unixSocketConfigured bool
	insecureDevMode      bool
	authLifecycleMu      sync.Mutex
	durabilityMu         sync.RWMutex
	durabilityClosed     bool
	durabilityReason     string
	now                  func() time.Time
}

type stateStorePersister interface {
	SaveStateToFile(path string) error
}

type authStorePersister interface {
	SaveToFile(path string) error
	SaveToFileEncrypted(path string, key []byte) error
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
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := getenv("PROMPTLOCK_CONFIG", "/etc/promptlock/config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if err := applyEnvOverrides(&cfg); err != nil {
		return err
	}
	if err := validateSocketConfig(cfg); err != nil {
		return err
	}
	if err := loadStartupSOPSEnv(cfg); err != nil {
		return err
	}
	if err := validateSecretSourceSafety(cfg); err != nil {
		return err
	}
	if err := validateStateStoreSafety(cfg); err != nil {
		return err
	}

	store := memory.NewStore()
	secretStore := any(store).(ports.SecretStore)
	requestStore := any(store).(ports.RequestStore)
	leaseStore := any(store).(ports.LeaseStore)
	envPathRoot := strings.TrimSpace(getenv("PROMPTLOCK_ENV_PATH_ROOT", ""))
	if envPathRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve env-path root from working directory: %w", err)
		}
		envPathRoot = cwd
	}
	envPathStore, err := envpath.New(envPathRoot)
	if err != nil {
		return fmt.Errorf("init env-path secret source root %q: %w", envPathRoot, err)
	}
	stateStoreFile := cfg.StateStoreFile
	var statePersister stateStorePersister = store
	sourceType := strings.ToLower(strings.TrimSpace(cfg.SecretSource.Type))
	switch sourceType {
	case "env":
		secretStore = envsecret.New(cfg.SecretSource.EnvPrefix)
	case "file":
		fs, err := filesecret.New(cfg.SecretSource.FilePath)
		if err != nil {
			return fmt.Errorf("init file secret source: %w", err)
		}
		secretStore = fs
	case "external":
		fs, err := externalsecret.New(cfg.SecretSource.ExternalURL, cfg.SecretSource.ExternalAuthTokenEnv, cfg.SecretSource.ExternalTimeoutSec)
		if err != nil {
			return fmt.Errorf("init external secret source: %w", err)
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
	stateStoreType := normalizedStateStoreType(cfg)
	if stateStoreType == "external" {
		extStore, err := externalstate.New(cfg.StateStore.ExternalURL, cfg.StateStore.ExternalAuthTokenEnv, cfg.StateStore.ExternalTimeoutSec)
		if err != nil {
			return fmt.Errorf("init external state store: %w", err)
		}
		requestStore = extStore
		leaseStore = extStore
		statePersister = nil
		stateStoreFile = ""
	}
	persistenceLocks := make([]*fileLock, 0, 2)
	defer func() {
		for _, lock := range persistenceLocks {
			if err := lock.Close(); err != nil {
				log.Printf("WARNING: release persistence lock: %v", err)
			}
		}
	}()
	lockedPaths := map[string]struct{}{}
	lockPersistentPath := func(dataPath string) error {
		trimmed := strings.TrimSpace(dataPath)
		if trimmed == "" {
			return nil
		}
		lockPath := trimmed + ".lock"
		if _, exists := lockedPaths[lockPath]; exists {
			return nil
		}
		lock, err := acquireFileLock(lockPath)
		if err != nil {
			return fmt.Errorf("acquire persistence lock for %s: %w", trimmed, err)
		}
		persistenceLocks = append(persistenceLocks, lock)
		lockedPaths[lockPath] = struct{}{}
		return nil
	}
	if stateStoreType == "file" {
		if err := lockPersistentPath(stateStoreFile); err != nil {
			return err
		}
	}
	if cfg.Auth.EnableAuth {
		if err := lockPersistentPath(cfg.Auth.StoreFile); err != nil {
			return err
		}
	}
	if stateStoreType == "file" && strings.TrimSpace(stateStoreFile) != "" {
		if err := store.LoadStateFromFile(stateStoreFile); err != nil {
			return fmt.Errorf("load request/lease state store: %w", err)
		}
	}

	sink, err := audit.NewFileSink(cfg.AuditPath)
	if err != nil {
		return err
	}
	defer sink.Close()

	newReq := func() string { return mustSecureToken("req_") }
	newLease := func() string { return mustSecureToken("lease_") }

	svc := app.Service{
		Policy: cfg.ToPolicy(),
		RequestPolicy: app.RequestPolicy{
			IdenticalRequestCooldown: time.Duration(cfg.RequestPolicy.IdenticalRequestCooldownSeconds) * time.Second,
			MaxPendingPerAgent:       cfg.RequestPolicy.MaxPendingPerAgent,
			EnableActiveLeaseReuse:   cfg.RequestPolicy.EnableActiveLeaseReuse,
		},
		Requests:       requestStore,
		Leases:         leaseStore,
		Secrets:        secretStore,
		EnvPathSecrets: envPathStore,
		Audit:          sink,
		Now:            func() time.Time { return time.Now().UTC() },
		NewRequestID:   newReq,
		NewLeaseTok:    newLease,
	}

	if err := validateSecurityProfile(cfg, getenv("PROMPTLOCK_ALLOW_INSECURE_PROFILE", "")); err != nil {
		return err
	}
	if err := validateDeploymentMode(cfg, getenv("PROMPTLOCK_ALLOW_DEV_PROFILE", "")); err != nil {
		return err
	}
	if cfg.Auth.EnableAuth && cfg.Auth.OperatorToken == "" {
		return fmt.Errorf("auth enabled but operator_token is empty")
	}
	allowInsecureTCP := getenv("PROMPTLOCK_ALLOW_INSECURE_TCP", "")
	allowInsecureNoAuthTCP := getenv("PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP", "")
	if err := validateTransportSafety(cfg, allowInsecureTCP, allowInsecureNoAuthTCP); err != nil {
		return err
	}
	insecureTCPOverride := cfg.Auth.EnableAuth && !cfg.UsesUnixSocketTransport() && !isLocalAddress(cfg.Address) && allowInsecureTCP == "1"
	insecureNoAuthTCPOverride := !cfg.Auth.EnableAuth && !cfg.UsesUnixSocketTransport() && !isLocalAddress(cfg.Address) && allowInsecureNoAuthTCP == "1"
	insecureDevMode := isInsecureDevMode(cfg)
	if insecureTCPOverride {
		log.Printf("WARNING: insecure TCP override enabled (PROMPTLOCK_ALLOW_INSECURE_TCP=1) on %s", cfg.Address)
	}
	if insecureNoAuthTCPOverride {
		log.Printf("WARNING: unauthenticated non-local TCP override enabled (PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1) on %s", cfg.Address)
	}
	if insecureDevMode {
		log.Printf("WARNING: insecure dev mode enabled (auth disabled + plaintext secret return enabled)")
	}
	authStore := auth.NewStore()
	authStoreKey, err := resolveAuthStoreEncryptionKey(cfg)
	if err != nil {
		return err
	}
	if cfg.Auth.EnableAuth && strings.TrimSpace(cfg.Auth.StoreFile) != "" {
		var err error
		if len(authStoreKey) > 0 {
			err = authStore.LoadFromFileEncrypted(cfg.Auth.StoreFile, authStoreKey)
		} else {
			err = authStore.LoadFromFile(cfg.Auth.StoreFile)
		}
		if err != nil {
			return fmt.Errorf("load auth store: %w", err)
		}
	}
	policyEngine := app.NewDefaultControlPlanePolicy(cfg.ExecutionPolicy, cfg.HostOpsPolicy, cfg.NetworkEgressPolicy)
	s := &server{svc: svc, intents: cfg.Intents, authEnabled: cfg.Auth.EnableAuth, authCfg: cfg.Auth, execPolicy: cfg.ExecutionPolicy, hostOpsPolicy: cfg.HostOpsPolicy, networkEgressPolicy: cfg.NetworkEgressPolicy, securityProfile: strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)), authStore: authStore, authStorePersister: authStore, authStoreFile: cfg.Auth.StoreFile, authStoreKey: authStoreKey, stateStoreFile: stateStoreFile, stateStorePersister: statePersister, authLimiter: newAuthRateLimiter(cfg.Auth), policyEngine: policyEngine, unixSocketConfigured: cfg.UsesUnixSocketTransport(), insecureDevMode: insecureDevMode, now: func() time.Time { return time.Now().UTC() }}
	s.svc.MutationLock = &sync.Mutex{}
	s.ensureRequestLeaseStateCommitter()
	s.svc.AuditFailureHandler = func(err error) error {
		return s.closeDurabilityGate("audit", err)
	}
	if strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)) == "hardened" && strings.EqualFold(strings.TrimSpace(cfg.SecretSource.Type), "in_memory") {
		log.Printf("WARNING: hardened profile using in_memory secret source (set secret_source.type=env or external backend)")
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "startup_inmemory_secret_source_warning", Timestamp: s.now(), ActorType: "system", ActorID: "promptlockd"})
	}
	if insecureTCPOverride {
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "startup_insecure_tcp_override", Timestamp: s.now(), ActorType: "system", ActorID: "promptlockd", Metadata: map[string]string{"address": cfg.Address}})
	}
	if insecureNoAuthTCPOverride {
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "startup_insecure_noauth_tcp_override", Timestamp: s.now(), ActorType: "system", ActorID: "promptlockd", Metadata: map[string]string{"address": cfg.Address}})
	}
	if insecureDevMode {
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "startup_insecure_dev_mode_warning", Timestamp: s.now(), ActorType: "system", ActorID: "promptlockd"})
	}
	if cfg.Auth.EnableAuth && strings.TrimSpace(cfg.Auth.StoreFile) != "" && len(authStoreKey) == 0 {
		log.Printf("WARNING: auth store persistence is not encrypted (set %s for encryption key in non-dev use)", cfg.Auth.StoreEncryptionKeyEnv)
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "startup_auth_store_plaintext_warning", Timestamp: s.now(), ActorType: "system", ActorID: "promptlockd", Metadata: map[string]string{"store_file": cfg.Auth.StoreFile}})
	}
	if cfg.Auth.EnableAuth {
		startAuthCleanupLoop(s)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveConfiguredListeners(ctx, cfg, s)
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
	a := strings.TrimSpace(addr)
	if a == "" {
		return false
	}
	host := a
	if h, _, err := net.SplitHostPort(a); err == nil {
		host = h
	} else if strings.HasPrefix(a, "[") && strings.HasSuffix(a, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(a, "["), "]")
	}
	host = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")))
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func validateSecretSourceSafety(cfg config.Config) error {
	src := strings.ToLower(strings.TrimSpace(cfg.SecretSource.Type))
	if src == "" {
		src = "in_memory"
	}
	if src != "in_memory" && src != "env" && src != "file" && src != "external" {
		return fmt.Errorf("unsupported secret_source.type %q (supported: in_memory, env, file, external)", cfg.SecretSource.Type)
	}
	if src == "file" && strings.TrimSpace(cfg.SecretSource.FilePath) == "" {
		return fmt.Errorf("secret_source.type=file requires secret_source.file_path")
	}
	if src == "external" {
		if strings.TrimSpace(cfg.SecretSource.ExternalURL) == "" {
			return fmt.Errorf("secret_source.type=external requires secret_source.external_url")
		}
		if strings.TrimSpace(cfg.SecretSource.ExternalAuthTokenEnv) == "" {
			return fmt.Errorf("secret_source.type=external requires secret_source.external_auth_token_env")
		}
		u, err := url.Parse(strings.TrimSpace(cfg.SecretSource.ExternalURL))
		if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
			return fmt.Errorf("secret_source.external_url is invalid: %q", cfg.SecretSource.ExternalURL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("secret_source.external_url scheme must be http or https")
		}
	}
	if strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)) == "hardened" && src == "in_memory" {
		mode := strings.ToLower(strings.TrimSpace(cfg.SecretSource.InMemoryHardened))
		if mode == "fail" {
			return fmt.Errorf("hardened profile with in_memory secret_source is disallowed (secret_source.in_memory_hardened=fail)")
		}
	}
	return nil
}

func normalizedStateStoreType(cfg config.Config) string {
	mode := strings.ToLower(strings.TrimSpace(cfg.StateStore.Type))
	if mode == "" {
		return "file"
	}
	return mode
}

func validateStateStoreSafety(cfg config.Config) error {
	mode := normalizedStateStoreType(cfg)
	switch mode {
	case "file":
		return nil
	case "external":
		if strings.TrimSpace(cfg.StateStore.ExternalURL) == "" {
			return fmt.Errorf("state_store.type=external requires state_store.external_url")
		}
		if strings.TrimSpace(cfg.StateStore.ExternalAuthTokenEnv) == "" {
			return fmt.Errorf("state_store.type=external requires state_store.external_auth_token_env")
		}
		u, err := url.Parse(strings.TrimSpace(cfg.StateStore.ExternalURL))
		if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
			return fmt.Errorf("state_store.external_url is invalid: %q", cfg.StateStore.ExternalURL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("state_store.external_url scheme must be http or https")
		}
		return nil
	default:
		return fmt.Errorf("unsupported state_store.type %q (supported: file, external)", cfg.StateStore.Type)
	}
}

func validateSocketConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.UnixSocket) != "" && (strings.TrimSpace(cfg.AgentUnixSocket) != "" || strings.TrimSpace(cfg.OperatorUnixSocket) != "") {
		return fmt.Errorf("legacy unix_socket cannot be combined with agent_unix_socket or operator_unix_socket")
	}
	agentPath := strings.TrimSpace(cfg.AgentUnixSocket)
	operatorPath := strings.TrimSpace(cfg.OperatorUnixSocket)
	if agentPath != "" && operatorPath != "" && agentPath == operatorPath {
		return fmt.Errorf("agent_unix_socket and operator_unix_socket must be different paths")
	}
	return nil
}

func validateTransportSafety(cfg config.Config, allowInsecureAuthTCP, allowInsecureNoAuthTCP string) error {
	if cfg.Auth.EnableAuth && !cfg.UsesUnixSocketTransport() && !isLocalAddress(cfg.Address) && allowInsecureAuthTCP != "1" {
		return fmt.Errorf("auth enabled on non-local TCP without unix socket transport; set unix_socket or agent_unix_socket/operator_unix_socket, or PROMPTLOCK_ALLOW_INSECURE_TCP=1")
	}
	if !cfg.Auth.EnableAuth && !cfg.UsesUnixSocketTransport() && !isLocalAddress(cfg.Address) && allowInsecureNoAuthTCP != "1" {
		return fmt.Errorf("auth disabled on non-local TCP without unix socket transport; enable auth, set unix_socket or agent_unix_socket/operator_unix_socket, or PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1")
	}
	return nil
}

func serveConfiguredListeners(ctx context.Context, cfg config.Config, s *server) error {
	switch {
	case cfg.UsesLegacyUnixSocket():
		mux := http.NewServeMux()
		s.registerLegacyRoutesTo(mux)
		return serveUnixSocket(ctx, cfg.UnixSocket, 0o600, mux, "unix socket")
	case cfg.UsesDualUnixSockets():
		if strings.TrimSpace(cfg.OperatorUnixSocket) == "" || strings.TrimSpace(cfg.AgentUnixSocket) == "" {
			return fmt.Errorf("agent_unix_socket and operator_unix_socket are both required in dual-socket mode")
		}
		agentMux := http.NewServeMux()
		s.registerAgentRoutesTo(agentMux)
		operatorMux := http.NewServeMux()
		s.registerOperatorRoutesTo(operatorMux)
		return serveUnixSockets(ctx, []unixSocketTarget{
			{Path: cfg.AgentUnixSocket, Mode: 0o660, Handler: agentMux, Label: "agent unix socket"},
			{Path: cfg.OperatorUnixSocket, Mode: 0o600, Handler: operatorMux, Label: "operator unix socket"},
		})
	default:
		mux := http.NewServeMux()
		s.registerLegacyRoutesTo(mux)
		ln, err := net.Listen("tcp", cfg.Address)
		if err != nil {
			return err
		}
		srv := &http.Server{Handler: mux}
		log.Printf("promptlock listening on %s", cfg.Address)
		return serveHTTPListener(ctx, srv, ln)
	}
}

type unixSocketTarget struct {
	Path    string
	Mode    os.FileMode
	Handler http.Handler
	Label   string
}

func serveUnixSocket(ctx context.Context, path string, mode os.FileMode, handler http.Handler, label string) error {
	return serveUnixSockets(ctx, []unixSocketTarget{{Path: path, Mode: mode, Handler: handler, Label: label}})
}

func serveUnixSockets(ctx context.Context, targets []unixSocketTarget) error {
	type cleanupFunc func()
	cleanups := make([]cleanupFunc, 0, len(targets))
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()
	servers := make([]*http.Server, 0, len(targets))
	errCh := make(chan error, len(targets))
	for _, target := range targets {
		target := target
		_ = os.Remove(target.Path)
		ln, err := net.Listen("unix", target.Path)
		if err != nil {
			return err
		}
		if err := os.Chmod(target.Path, target.Mode); err != nil {
			_ = ln.Close()
			_ = os.Remove(target.Path)
			return err
		}
		cleanups = append(cleanups, func() {
			_ = ln.Close()
			_ = os.Remove(target.Path)
		})
		log.Printf("promptlock listening on %s %s", target.Label, target.Path)
		srv := &http.Server{Handler: target.Handler}
		servers = append(servers, srv)
		go func(srv *http.Server) {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
				errCh <- fmt.Errorf("listener %s failed: %w", target.Path, err)
			}
		}(srv)
	}
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var shutdownErr error
		for _, srv := range servers {
			if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) && shutdownErr == nil {
				shutdownErr = err
			}
		}
		return shutdownErr
	}
}

func serveHTTPListener(ctx context.Context, srv *http.Server, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
			return err
		}
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}
}

func isInsecureDevMode(cfg config.Config) bool {
	return !cfg.Auth.EnableAuth && cfg.Auth.AllowPlaintextSecretReturn
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

func validateDeploymentMode(cfg config.Config, allowDevProfile string) error {
	profile := strings.TrimSpace(strings.ToLower(cfg.SecurityProfile))
	if profile == "" {
		profile = "dev"
	}
	if profile == "dev" && allowDevProfile != "1" {
		return fmt.Errorf("security_profile=dev is disabled by default; set PROMPTLOCK_ALLOW_DEV_PROFILE=1 for local testing or use security_profile=hardened")
	}
	if profile != "dev" {
		mode := normalizedStateStoreType(cfg)
		switch mode {
		case "file":
			if strings.TrimSpace(cfg.StateStoreFile) == "" {
				return fmt.Errorf("non-dev profile requires state_store_file for durable request/lease state when state_store.type=file")
			}
		case "external":
			extURL := strings.TrimSpace(cfg.StateStore.ExternalURL)
			if extURL == "" {
				return fmt.Errorf("non-dev profile with state_store.type=external requires state_store.external_url")
			}
			u, err := url.Parse(extURL)
			if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
				return fmt.Errorf("state_store.external_url is invalid: %q", cfg.StateStore.ExternalURL)
			}
			if u.Scheme != "https" {
				return fmt.Errorf("non-dev profile requires https state_store.external_url when state_store.type=external")
			}
			tokenEnv := strings.TrimSpace(cfg.StateStore.ExternalAuthTokenEnv)
			if tokenEnv == "" {
				return fmt.Errorf("non-dev profile with state_store.type=external requires state_store.external_auth_token_env")
			}
			if strings.TrimSpace(os.Getenv(tokenEnv)) == "" {
				return fmt.Errorf("state_store.type=external requires auth token env %s to be set in %s profile", tokenEnv, profile)
			}
		default:
			return fmt.Errorf("unsupported state_store.type %q (supported: file, external)", cfg.StateStore.Type)
		}
		if strings.TrimSpace(cfg.Auth.StoreFile) == "" {
			return fmt.Errorf("non-dev profile requires auth.store_file for durable auth state")
		}
		src := strings.ToLower(strings.TrimSpace(cfg.SecretSource.Type))
		if src == "" {
			src = "in_memory"
		}
		if src == "in_memory" {
			return fmt.Errorf("non-dev profile requires secret_source.type of env, file, or external (in_memory is not allowed)")
		}
		if src == "external" {
			extURL := strings.TrimSpace(cfg.SecretSource.ExternalURL)
			if extURL == "" {
				return fmt.Errorf("non-dev profile with secret_source.type=external requires secret_source.external_url")
			}
			u, err := url.Parse(extURL)
			if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
				return fmt.Errorf("secret_source.external_url is invalid: %q", cfg.SecretSource.ExternalURL)
			}
			if u.Scheme != "https" {
				return fmt.Errorf("non-dev profile requires https secret_source.external_url when secret_source.type=external")
			}
			tokenEnv := strings.TrimSpace(cfg.SecretSource.ExternalAuthTokenEnv)
			if tokenEnv == "" {
				return fmt.Errorf("non-dev profile with secret_source.type=external requires secret_source.external_auth_token_env")
			}
			if strings.TrimSpace(os.Getenv(tokenEnv)) == "" {
				return fmt.Errorf("secret_source.type=external requires auth token env %s to be set in %s profile", tokenEnv, profile)
			}
		}
	}
	if profile != "dev" && cfg.Auth.EnableAuth && strings.TrimSpace(cfg.Auth.StoreFile) != "" {
		keyEnv := strings.TrimSpace(cfg.Auth.StoreEncryptionKeyEnv)
		if keyEnv == "" {
			keyEnv = "PROMPTLOCK_AUTH_STORE_KEY"
		}
		if strings.TrimSpace(os.Getenv(keyEnv)) == "" {
			return fmt.Errorf("auth.store_file requires encrypted persistence in %s profile; set %s", profile, keyEnv)
		}
	}
	return nil
}

func resolveAuthStoreEncryptionKey(cfg config.Config) ([]byte, error) {
	keyEnv := strings.TrimSpace(cfg.Auth.StoreEncryptionKeyEnv)
	if keyEnv == "" {
		keyEnv = "PROMPTLOCK_AUTH_STORE_KEY"
	}
	v := strings.TrimSpace(os.Getenv(keyEnv))
	if v == "" {
		return nil, nil
	}
	if len(v) < 16 {
		return nil, fmt.Errorf("%s must be at least 16 characters", keyEnv)
	}
	return []byte(v), nil
}
