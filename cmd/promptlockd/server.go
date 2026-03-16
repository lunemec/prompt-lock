package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
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
	executeUseCase       app.ExecuteWithLeaseUseCase
	hostDockerUseCase    app.HostDockerExecuteUseCase
	ambientProcessEnv    []string
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
