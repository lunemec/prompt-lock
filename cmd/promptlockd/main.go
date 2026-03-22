package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/audit"
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
	envPathStore, envPathRoot, err := newEnvPathSecretStore(cfg, os.Getwd)
	if err != nil {
		return err
	}
	_, envPathDisabled := envPathStore.(envPathDisabledStore)
	if strings.TrimSpace(getenv("PROMPTLOCK_ENV_PATH_ROOT", "")) == "" {
		switch normalizedSecurityProfile(cfg) {
		case "dev":
			log.Printf("WARNING: using working directory as env-path root in dev profile: %s", envPathRoot)
		default:
			log.Printf("INFO: env_path requests disabled until PROMPTLOCK_ENV_PATH_ROOT is set")
		}
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
	s := &server{svc: svc, intents: cfg.Intents, authEnabled: cfg.Auth.EnableAuth, authCfg: cfg.Auth, execPolicy: cfg.ExecutionPolicy, hostOpsPolicy: cfg.HostOpsPolicy, networkEgressPolicy: cfg.NetworkEgressPolicy, securityProfile: strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)), authStore: authStore, authStorePersister: authStore, authStoreFile: cfg.Auth.StoreFile, authStoreKey: authStoreKey, stateStoreFile: stateStoreFile, stateStorePersister: statePersister, authLimiter: newAuthRateLimiter(cfg.Auth), policyEngine: policyEngine, ambientProcessEnv: os.Environ(), unixSocketConfigured: cfg.UsesUnixSocketTransport(), agentBridgeAddress: strings.TrimSpace(cfg.AgentBridgeAddress), envPathEnabled: !envPathDisabled, insecureDevMode: insecureDevMode, now: func() time.Time { return time.Now().UTC() }}
	s.svc.MutationLock = &sync.Mutex{}
	s.ensureRequestLeaseStateCommitter()
	s.svc.AuditFailureHandler = func(err error) error {
		return s.closeDurabilityGate("audit", err)
	}
	configureControlPlaneUseCases(s)
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
