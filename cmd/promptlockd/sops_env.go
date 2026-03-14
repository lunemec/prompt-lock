package main

import (
	"os"
	"strings"

	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/sopsenv"
)

var loadPromptlockSOPSEnvFile = sopsenv.LoadFromFile

func loadStartupSOPSEnv(cfg config.Config) error {
	path := strings.TrimSpace(os.Getenv(sopsenv.DefaultEnvFileEnv))
	if path == "" {
		return nil
	}
	required := make([]string, 0, 1)
	profile := strings.ToLower(strings.TrimSpace(cfg.SecurityProfile))
	if profile == "" {
		profile = "dev"
	}
	if profile != "dev" && cfg.Auth.EnableAuth && strings.TrimSpace(cfg.Auth.StoreFile) != "" {
		keyEnv := strings.TrimSpace(cfg.Auth.StoreEncryptionKeyEnv)
		if keyEnv == "" {
			keyEnv = "PROMPTLOCK_AUTH_STORE_KEY"
		}
		required = append(required, keyEnv)
	}
	return loadPromptlockSOPSEnvFile(path, required)
}
