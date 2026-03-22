package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lunemec/promptlock/internal/config"
)

func resolvedWorkspaceSetupConfigPath(explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	layout, ok := detectWorkspaceSetupLayout()
	if !ok {
		return ""
	}
	return layout.ConfigPath
}

func detectWorkspaceSetupLayout() (workspaceSetupLayout, bool) {
	cwd, err := setupGetwd()
	if err != nil {
		return workspaceSetupLayout{}, false
	}
	layout, err := buildWorkspaceSetupLayout(cwd, "")
	if err != nil {
		return workspaceSetupLayout{}, false
	}
	if !fileExists(layout.ConfigPath) || !fileExists(layout.EnvPath) {
		return workspaceSetupLayout{}, false
	}
	return layout, true
}

func loadWorkspaceSetupConfig(configPath string) (config.Config, bool) {
	trimmed := strings.TrimSpace(configPath)
	if trimmed == "" {
		return config.Config{}, false
	}
	cfg, err := config.Load(trimmed)
	if err != nil {
		return config.Config{}, false
	}
	return cfg, true
}

func workspaceSetupBrokerSocket(role brokerRole, configPath string) string {
	cfg, ok := loadWorkspaceSetupConfig(resolvedWorkspaceSetupConfigPath(configPath))
	if !ok {
		return ""
	}
	switch role {
	case brokerRoleOperator:
		if trimmed := strings.TrimSpace(cfg.OperatorUnixSocket); trimmed != "" {
			return trimmed
		}
	case brokerRoleAgent:
		if trimmed := strings.TrimSpace(cfg.AgentUnixSocket); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(cfg.UnixSocket)
}

func defaultOperatorToken() string {
	if token := strings.TrimSpace(os.Getenv("PROMPTLOCK_OPERATOR_TOKEN")); token != "" {
		return token
	}
	cfg, ok := loadWorkspaceSetupConfig(resolvedWorkspaceSetupConfigPath(os.Getenv("PROMPTLOCK_CONFIG")))
	if !ok {
		return ""
	}
	return strings.TrimSpace(cfg.Auth.OperatorToken)
}

func workspaceSetupEnvFilePath(configPath string) string {
	if trimmed := strings.TrimSpace(configPath); trimmed != "" {
		return filepath.Join(filepath.Dir(trimmed), "instance.env")
	}
	layout, ok := detectWorkspaceSetupLayout()
	if !ok {
		return ""
	}
	return layout.EnvPath
}

func loadWorkspaceSetupEnvExports(configPath string) map[string]string {
	envPath := workspaceSetupEnvFilePath(configPath)
	if strings.TrimSpace(envPath) == "" {
		return nil
	}
	body, err := os.ReadFile(envPath)
	if err != nil {
		return nil
	}
	exports := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		name, value, ok := parseSetupExportLine(line)
		if !ok {
			continue
		}
		exports[name] = value
	}
	if len(exports) == 0 {
		return nil
	}
	return exports
}

func parseSetupExportLine(line string) (string, string, bool) {
	name, ok := setupExportName(strings.TrimSpace(line))
	if !ok {
		return "", "", false
	}
	_, rawValue, found := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
	if !found {
		return "", "", false
	}
	value, ok := parseSetupExportValue(rawValue)
	if !ok {
		return "", "", false
	}
	return name, value, true
}

func parseSetupExportValue(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return "", true
	case strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'") && len(trimmed) >= 2:
		inner := trimmed[1 : len(trimmed)-1]
		return strings.ReplaceAll(inner, `'\''`, `'`), true
	case strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) && len(trimmed) >= 2:
		inner := trimmed[1 : len(trimmed)-1]
		if strings.Contains(inner, "${") {
			return "", false
		}
		return inner, true
	default:
		if strings.ContainsAny(trimmed, "$`") {
			return "", false
		}
		return trimmed, true
	}
}

func mergeMissingEnv(env []string, exports map[string]string) []string {
	if len(exports) == 0 {
		return env
	}
	seen := map[string]bool{}
	for _, kv := range env {
		name, value, found := strings.Cut(kv, "=")
		if !found {
			continue
		}
		seen[strings.TrimSpace(name)] = strings.TrimSpace(value) != ""
	}
	for name, value := range exports {
		if present, ok := seen[name]; ok && present {
			continue
		}
		env = append(env, name+"="+value)
	}
	return env
}
