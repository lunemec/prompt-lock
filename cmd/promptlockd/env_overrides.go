package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lunemec/promptlock/internal/config"
)

func applyEnvOverrides(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
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
	if v := os.Getenv("PROMPTLOCK_AGENT_UNIX_SOCKET"); v != "" {
		cfg.AgentUnixSocket = v
	}
	if v := os.Getenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET"); v != "" {
		cfg.OperatorUnixSocket = v
	}
	if v := os.Getenv("PROMPTLOCK_STATE_STORE_FILE"); v != "" {
		cfg.StateStoreFile = v
	}
	if v := os.Getenv("PROMPTLOCK_OPERATOR_TOKEN"); v != "" {
		cfg.Auth.OperatorToken = v
	}
	if v := os.Getenv("PROMPTLOCK_STATE_STORE_TYPE"); v != "" {
		cfg.StateStore.Type = v
	}
	if v := os.Getenv("PROMPTLOCK_STATE_STORE_EXTERNAL_URL"); v != "" {
		cfg.StateStore.ExternalURL = v
	}
	if v := os.Getenv("PROMPTLOCK_STATE_STORE_EXTERNAL_AUTH_TOKEN_ENV"); v != "" {
		cfg.StateStore.ExternalAuthTokenEnv = v
	}
	if v := os.Getenv("PROMPTLOCK_STATE_STORE_EXTERNAL_TIMEOUT_SEC"); strings.TrimSpace(v) != "" {
		timeoutSeconds, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid PROMPTLOCK_STATE_STORE_EXTERNAL_TIMEOUT_SEC: %w", err)
		}
		cfg.StateStore.ExternalTimeoutSec = timeoutSeconds
	}
	return nil
}
