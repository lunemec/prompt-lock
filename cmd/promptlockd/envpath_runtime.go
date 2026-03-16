package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lunemec/promptlock/internal/adapters/envpath"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type envPathDisabledStore struct {
	reason string
}

func (s envPathDisabledStore) Canonicalize(string) (string, error) {
	return "", errors.New(strings.TrimSpace(s.reason))
}

func (s envPathDisabledStore) Resolve(string, []string) (map[string]string, string, error) {
	return nil, "", errors.New(strings.TrimSpace(s.reason))
}

func newEnvPathSecretStore(cfg config.Config, getwd func() (string, error)) (ports.EnvPathSecretStore, string, error) {
	if getwd == nil {
		getwd = os.Getwd
	}
	configuredRoot := strings.TrimSpace(os.Getenv("PROMPTLOCK_ENV_PATH_ROOT"))
	if configuredRoot == "" && normalizedSecurityProfile(cfg) != "dev" {
		return envPathDisabledStore{reason: "env_path requires PROMPTLOCK_ENV_PATH_ROOT in non-dev profiles"}, "", nil
	}
	if configuredRoot == "" {
		cwd, err := getwd()
		if err != nil {
			return nil, "", fmt.Errorf("resolve env-path root from working directory: %w", err)
		}
		configuredRoot = cwd
	}
	store, err := envpath.New(configuredRoot)
	if err != nil {
		return nil, configuredRoot, fmt.Errorf("init env-path secret source root %q: %w", configuredRoot, err)
	}
	return store, configuredRoot, nil
}
