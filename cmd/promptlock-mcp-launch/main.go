package main

import (
	"fmt"
	"github.com/lunemec/promptlock/internal/mcplaunchenv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	env, err := normalizedLaunchEnv(os.Environ())
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	target, err := resolvePromptlockMCPBinary(os.Executable, exec.LookPath, fileExists)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	cmd := exec.Command(target, os.Args[1:]...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func normalizedLaunchEnv(base []string) ([]string, error) {
	envMap, order := envMapWithOrder(base)
	if err := mergeWrapperEnvFile(envMap, &order, os.ReadFile); err != nil {
		return nil, err
	}

	session := strings.TrimSpace(firstNonEmpty(
		envMap["PROMPTLOCK_WRAPPER_SESSION_TOKEN"],
		envMap["PROMPTLOCK_SESSION_TOKEN"],
	))
	if session == "" {
		return nil, fmt.Errorf("PROMPTLOCK_SESSION_TOKEN is required; run inside promptlock auth docker-run or export a current session token")
	}

	agentUnix := strings.TrimSpace(firstNonEmpty(
		envMap["PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET"],
		envMap["PROMPTLOCK_AGENT_UNIX_SOCKET"],
	))
	brokerURL := strings.TrimSpace(firstNonEmpty(
		envMap["PROMPTLOCK_WRAPPER_BROKER_URL"],
		envMap["PROMPTLOCK_BROKER_URL"],
	))
	if agentUnix == "" && brokerURL == "" {
		return nil, fmt.Errorf("PromptLock agent transport is required; expected PROMPTLOCK_AGENT_UNIX_SOCKET or PROMPTLOCK_BROKER_URL")
	}

	setEnv(envMap, &order, "PROMPTLOCK_SESSION_TOKEN", session)
	if wrapperSession := strings.TrimSpace(envMap["PROMPTLOCK_WRAPPER_SESSION_TOKEN"]); wrapperSession != "" {
		setEnv(envMap, &order, "PROMPTLOCK_WRAPPER_SESSION_TOKEN", wrapperSession)
	}
	if agentUnix != "" {
		setEnv(envMap, &order, "PROMPTLOCK_AGENT_UNIX_SOCKET", agentUnix)
	}
	if brokerURL != "" {
		setEnv(envMap, &order, "PROMPTLOCK_BROKER_URL", brokerURL)
	}
	if wrapperUnix := strings.TrimSpace(envMap["PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET"]); wrapperUnix != "" {
		setEnv(envMap, &order, "PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET", wrapperUnix)
	}
	if wrapperURL := strings.TrimSpace(envMap["PROMPTLOCK_WRAPPER_BROKER_URL"]); wrapperURL != "" {
		setEnv(envMap, &order, "PROMPTLOCK_WRAPPER_BROKER_URL", wrapperURL)
	}

	if strings.TrimSpace(envMap["PROMPTLOCK_AGENT_ID"]) == "" {
		if wrapperAgentID := strings.TrimSpace(envMap["PROMPTLOCK_WRAPPER_AGENT_ID"]); wrapperAgentID != "" {
			setEnv(envMap, &order, "PROMPTLOCK_AGENT_ID", wrapperAgentID)
		}
	}
	if strings.TrimSpace(envMap["PROMPTLOCK_TASK_ID"]) == "" {
		if wrapperTaskID := strings.TrimSpace(envMap["PROMPTLOCK_WRAPPER_TASK_ID"]); wrapperTaskID != "" {
			setEnv(envMap, &order, "PROMPTLOCK_TASK_ID", wrapperTaskID)
		} else {
			setEnv(envMap, &order, "PROMPTLOCK_TASK_ID", "mcp-task")
		}
	}

	return envListFromOrder(envMap, order), nil
}

func mergeWrapperEnvFile(envMap map[string]string, order *[]string, readFile func(string) ([]byte, error)) error {
	path := mcplaunchenv.FilePathFromEnv(func(key string) string {
		return envMap[key]
	})
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read PromptLock MCP env file %s: %w", path, err)
	}
	parsed, err := parseWrapperEnvFile(data)
	if err != nil {
		return fmt.Errorf("failed to parse PromptLock MCP env file %s: %w", path, err)
	}
	for name, value := range parsed {
		if strings.TrimSpace(envMap[name]) != "" {
			continue
		}
		setEnv(envMap, order, name, value)
	}
	return nil
}

func defaultWrapperMCPEnvFilePath() string {
	return mcplaunchenv.DefaultFilePath
}

func parseWrapperEnvFile(data []byte) (map[string]string, error) {
	return mcplaunchenv.Parse(data)
}

func resolvePromptlockMCPBinary(executable func() (string, error), lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	if override := strings.TrimSpace(os.Getenv("PROMPTLOCK_MCP_BIN")); override != "" {
		if exists(override) {
			return override, nil
		}
		return "", fmt.Errorf("PROMPTLOCK_MCP_BIN points to missing executable %s", override)
	}
	if executable != nil {
		if exePath, err := executable(); err == nil {
			sibling := filepath.Join(filepath.Dir(exePath), "promptlock-mcp")
			if exists(sibling) {
				return sibling, nil
			}
		}
	}
	if lookPath != nil {
		if target, err := lookPath("promptlock-mcp"); err == nil {
			return target, nil
		}
	}
	return "", fmt.Errorf("promptlock-mcp launcher could not find promptlock-mcp; install it alongside promptlock-mcp-launch or set PROMPTLOCK_MCP_BIN")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func envMapWithOrder(base []string) (map[string]string, []string) {
	values := make(map[string]string, len(base))
	order := make([]string, 0, len(base))
	for _, entry := range base {
		if entry == "" {
			continue
		}
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		if _, seen := values[name]; !seen {
			order = append(order, name)
		}
		values[name] = value
	}
	return values, order
}

func envListFromOrder(values map[string]string, order []string) []string {
	out := make([]string, 0, len(order))
	for _, name := range order {
		value, ok := values[name]
		if !ok {
			continue
		}
		out = append(out, name+"="+value)
	}
	return out
}

func setEnv(values map[string]string, order *[]string, name, value string) {
	if _, seen := values[name]; !seen {
		*order = append(*order, name)
	}
	values[name] = value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
