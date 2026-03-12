package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lunemec/promptlock/internal/adapters/envpath"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func (s *server) ensureEnvPathSecretStore() (ports.EnvPathSecretStore, error) {
	if s.svc.EnvPathSecrets != nil {
		return s.svc.EnvPathSecrets, nil
	}

	root := strings.TrimSpace(os.Getenv("PROMPTLOCK_ENV_PATH_ROOT"))
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve env-path root from working directory: %w", err)
		}
		root = cwd
	}

	store, err := envpath.New(root)
	if err != nil {
		return nil, fmt.Errorf("init env-path secret source root %q: %w", root, err)
	}
	s.svc.EnvPathSecrets = store
	return store, nil
}
