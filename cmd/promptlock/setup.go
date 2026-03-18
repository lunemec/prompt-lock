package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lunemec/promptlock/internal/config"
)

const (
	defaultSetupIntentName         = "run_tests"
	defaultSetupSecretName         = "github_token"
	defaultSetupAllowDomain        = "api.github.com"
	defaultSetupDemoSecretValue    = "demo_github_token_value"
	defaultSetupOutputSecurityMode = "raw"
)

var setupGetwd = os.Getwd
var setupRuntimeGOOS = runtime.GOOS
var setupRandomBytes = func(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

type workspaceSetupOptions struct {
	StateDir           string
	IntentName         string
	SecretName         string
	AllowDomain        string
	DemoSecretValue    string
	OutputSecurityMode string
}

type workspaceSetupLayout struct {
	WorkspaceRoot      string
	WorkspaceSlug      string
	WorkspaceID        string
	InstanceDir        string
	SocketDir          string
	ConfigPath         string
	EnvPath            string
	AuditPath          string
	StateStorePath     string
	AuthStorePath      string
	AgentSocketPath    string
	OperatorSocketPath string
	AgentBridgeAddress string
}

type workspaceSetupResult struct {
	Created            bool
	WorkspaceRoot      string
	InstanceDir        string
	SocketDir          string
	ConfigPath         string
	EnvPath            string
	AuditPath          string
	StateStorePath     string
	AuthStorePath      string
	AgentSocketPath    string
	OperatorSocketPath string
	AgentBridgeAddress string
	DockerBridgeURL    string
	IntentName         string
	SecretName         string
	OutputSecurityMode string
	AgentID            string
	ContainerID        string
	TaskID             string
	ImageName          string
}

func runSetup(args []string) {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("setup", flag.ContinueOnError)
		fs.String("state-dir", "", "optional host-side state root; defaults to XDG_STATE_HOME or ~/.local/state")
		fs.String("intent", defaultSetupIntentName, "default quickstart intent name")
		fs.String("secret-name", defaultSetupSecretName, "default quickstart secret name")
		fs.String("allow-domain", defaultSetupAllowDomain, "default egress allow domain for the quickstart intent")
		fs.String("demo-secret-value", defaultSetupDemoSecretValue, "local quickstart demo secret value written to the generated env file")
		fs.String("output-security-mode", defaultSetupOutputSecurityMode, "execution_policy.output_security_mode for the generated quickstart config")
		printFlagHelp(os.Stdout, setupHelpText(), fs)
		return
	}

	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	stateDir := fs.String("state-dir", "", "optional host-side state root; defaults to XDG_STATE_HOME or ~/.local/state")
	intentName := fs.String("intent", defaultSetupIntentName, "default quickstart intent name")
	secretName := fs.String("secret-name", defaultSetupSecretName, "default quickstart secret name")
	allowDomain := fs.String("allow-domain", defaultSetupAllowDomain, "default egress allow domain for the quickstart intent")
	demoSecret := fs.String("demo-secret-value", defaultSetupDemoSecretValue, "local quickstart demo secret value written to the generated env file")
	outputMode := fs.String("output-security-mode", defaultSetupOutputSecurityMode, "execution_policy.output_security_mode for the generated quickstart config")
	fs.Parse(args)

	cwd, err := setupGetwd()
	if err != nil {
		fatal(fmt.Errorf("resolve working directory: %w", err))
	}
	result, err := ensureWorkspaceSetup(cwd, workspaceSetupOptions{
		StateDir:           strings.TrimSpace(*stateDir),
		IntentName:         strings.TrimSpace(*intentName),
		SecretName:         strings.TrimSpace(*secretName),
		AllowDomain:        strings.TrimSpace(*allowDomain),
		DemoSecretValue:    *demoSecret,
		OutputSecurityMode: strings.TrimSpace(*outputMode),
	})
	if err != nil {
		fatal(err)
	}
	fmt.Print(renderWorkspaceSetupSummary(result))
}

func ensureWorkspaceSetup(cwd string, opts workspaceSetupOptions) (workspaceSetupResult, error) {
	layout, err := buildWorkspaceSetupLayout(cwd, opts.StateDir)
	if err != nil {
		return workspaceSetupResult{}, err
	}

	configExists := fileExists(layout.ConfigPath)
	envExists := fileExists(layout.EnvPath)
	otherStateExists := fileExists(layout.AuthStorePath) || fileExists(layout.StateStorePath) || fileExists(layout.AuditPath)
	if configExists || envExists || otherStateExists {
		switch {
		case configExists && envExists:
			if err := ensureWorkspaceSocketDir(layout.SocketDir); err != nil {
				return workspaceSetupResult{}, err
			}
			if _, err := refreshExistingWorkspaceSetupPaths(layout); err != nil {
				return workspaceSetupResult{}, err
			}
			return newWorkspaceSetupResult(layout, opts, false), nil
		default:
			return workspaceSetupResult{}, fmt.Errorf("incomplete existing workspace setup under %s; remove it manually before re-running promptlock setup", layout.InstanceDir)
		}
	}

	if err := os.MkdirAll(layout.InstanceDir, 0o700); err != nil {
		return workspaceSetupResult{}, fmt.Errorf("create workspace instance dir %s: %w", layout.InstanceDir, err)
	}
	if err := os.Chmod(layout.InstanceDir, 0o700); err != nil {
		return workspaceSetupResult{}, fmt.Errorf("chmod workspace instance dir %s: %w", layout.InstanceDir, err)
	}
	if err := ensureWorkspaceSocketDir(layout.SocketDir); err != nil {
		return workspaceSetupResult{}, err
	}

	operatorToken, err := generateSetupToken("op_", 16)
	if err != nil {
		return workspaceSetupResult{}, fmt.Errorf("generate operator token: %w", err)
	}
	authKey, err := generateSetupToken("", 32)
	if err != nil {
		return workspaceSetupResult{}, fmt.Errorf("generate auth store key: %w", err)
	}

	configBody, err := buildWorkspaceSetupConfig(layout, opts, operatorToken)
	if err != nil {
		return workspaceSetupResult{}, err
	}
	if err := writePrivateFile(layout.ConfigPath, configBody); err != nil {
		return workspaceSetupResult{}, fmt.Errorf("write setup config %s: %w", layout.ConfigPath, err)
	}

	envBody := buildWorkspaceSetupEnvFile(layout, opts, operatorToken, authKey)
	if err := writePrivateFile(layout.EnvPath, []byte(envBody)); err != nil {
		return workspaceSetupResult{}, fmt.Errorf("write setup env file %s: %w", layout.EnvPath, err)
	}

	return newWorkspaceSetupResult(layout, opts, true), nil
}

func buildWorkspaceSetupLayout(cwd, stateDir string) (workspaceSetupLayout, error) {
	if strings.TrimSpace(cwd) == "" {
		return workspaceSetupLayout{}, fmt.Errorf("working directory is required")
	}
	workspaceRoot, err := detectWorkspaceRoot(cwd)
	if err != nil {
		return workspaceSetupLayout{}, err
	}
	stateHome, err := promptlockStateHome(stateDir)
	if err != nil {
		return workspaceSetupLayout{}, err
	}

	slug := sanitizeWorkspaceSlug(filepath.Base(workspaceRoot))
	sum := sha256.Sum256([]byte(filepath.Clean(workspaceRoot)))
	shortHash := hex.EncodeToString(sum[:])[:10]
	workspaceID := slug + "-" + shortHash
	instanceDir := filepath.Join(stateHome, "promptlock", "workspaces", workspaceID)
	socketDir := workspaceSocketDir(stateHome, workspaceID)

	return workspaceSetupLayout{
		WorkspaceRoot:      workspaceRoot,
		WorkspaceSlug:      slug,
		WorkspaceID:        workspaceID,
		InstanceDir:        instanceDir,
		SocketDir:          socketDir,
		ConfigPath:         filepath.Join(instanceDir, "config.json"),
		EnvPath:            filepath.Join(instanceDir, "instance.env"),
		AuditPath:          filepath.Join(instanceDir, "audit.jsonl"),
		StateStorePath:     filepath.Join(instanceDir, "state-store.json"),
		AuthStorePath:      filepath.Join(instanceDir, "auth-store.json"),
		AgentSocketPath:    filepath.Join(socketDir, "agent.sock"),
		OperatorSocketPath: filepath.Join(socketDir, "operator.sock"),
		AgentBridgeAddress: config.DefaultAgentBridgeAddressForGOOS(setupRuntimeGOOS),
	}, nil
}

func detectWorkspaceRoot(start string) (string, error) {
	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path %s: %w", start, err)
	}
	current := filepath.Clean(absStart)
	for {
		if fileExists(filepath.Join(current, ".git")) || fileExists(filepath.Join(current, "go.mod")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return current, nil
		}
		current = parent
	}
}

func promptlockStateHome(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return "", fmt.Errorf("resolve state dir %s: %w", trimmed, err)
		}
		return filepath.Clean(abs), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
		abs, err := filepath.Abs(xdg)
		if err != nil {
			return "", fmt.Errorf("resolve XDG_STATE_HOME %s: %w", xdg, err)
		}
		return filepath.Clean(abs), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for state storage: %w", err)
	}
	return filepath.Join(home, ".local", "state"), nil
}

func workspaceSocketDir(stateHome, workspaceID string) string {
	if strings.EqualFold(strings.TrimSpace(setupRuntimeGOOS), "windows") {
		return filepath.Join(stateHome, "promptlock", "sockets", workspaceID)
	}
	return filepath.Join(string(os.PathSeparator), "tmp", "promptlock", workspaceID)
}

func sanitizeWorkspaceSlug(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return "workspace"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "workspace"
	}
	return out
}

func generateSetupToken(prefix string, byteLen int) (string, error) {
	b, err := setupRandomBytes(byteLen)
	if err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

func buildWorkspaceSetupConfig(layout workspaceSetupLayout, opts workspaceSetupOptions, operatorToken string) ([]byte, error) {
	intentName := defaultIfEmpty(opts.IntentName, defaultSetupIntentName)
	secretName := defaultIfEmpty(opts.SecretName, defaultSetupSecretName)
	allowDomain := defaultIfEmpty(opts.AllowDomain, defaultSetupAllowDomain)
	outputMode := defaultIfEmpty(opts.OutputSecurityMode, defaultSetupOutputSecurityMode)
	doc := map[string]any{
		"security_profile":     "hardened",
		"address":              "127.0.0.1:8765",
		"agent_unix_socket":    layout.AgentSocketPath,
		"operator_unix_socket": layout.OperatorSocketPath,
		"audit_path":           layout.AuditPath,
		"state_store_file":     layout.StateStorePath,
		"state_store": map[string]any{
			"type": "file",
		},
		"auth": map[string]any{
			"enable_auth":                   true,
			"operator_token":                operatorToken,
			"allow_plaintext_secret_return": false,
			"store_file":                    layout.AuthStorePath,
			"store_encryption_key_env":      "PROMPTLOCK_AUTH_STORE_KEY",
		},
		"secret_source": map[string]any{
			"type":               "env",
			"env_prefix":         "PROMPTLOCK_SECRET_",
			"in_memory_hardened": "fail",
		},
		"execution_policy": map[string]any{
			"output_security_mode": outputMode,
		},
		"network_egress_policy": map[string]any{
			"enabled":              true,
			"require_intent_match": true,
			"intent_allow_domains": map[string]any{
				intentName: []string{allowDomain},
			},
			"deny_substrings": []string{
				"169.254.169.254",
				"metadata.google.internal",
				"localhost",
				"127.0.0.1",
			},
		},
		"intents": map[string]any{
			intentName: []string{secretName},
		},
	}
	if strings.TrimSpace(layout.AgentBridgeAddress) != "" {
		doc["agent_bridge_address"] = layout.AgentBridgeAddress
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal setup config: %w", err)
	}
	return append(b, '\n'), nil
}

func buildWorkspaceSetupEnvFile(layout workspaceSetupLayout, opts workspaceSetupOptions, operatorToken, authKey string) string {
	secretName := defaultIfEmpty(opts.SecretName, defaultSetupSecretName)
	demoSecretValue := defaultIfEmpty(opts.DemoSecretValue, defaultSetupDemoSecretValue)
	secretEnvName := setupSecretEnvName(secretName)

	lines := []string{
		"# Generated by `promptlock setup` for a local hardened Docker quickstart.",
		"# This file stays outside the repo so supported config/state do not live in the agent workspace.",
		"# Replace the demo secret value before real use.",
		"export PROMPTLOCK_SETUP_WORKSPACE_ROOT=" + shellQuote(layout.WorkspaceRoot),
		"export PROMPTLOCK_SETUP_INSTANCE_DIR=" + shellQuote(layout.InstanceDir),
		"export PROMPTLOCK_SETUP_SOCKET_DIR=" + shellQuote(layout.SocketDir),
		"export PROMPTLOCK_CONFIG=" + shellQuote(layout.ConfigPath),
		"export PROMPTLOCK_AGENT_UNIX_SOCKET=" + shellQuote(layout.AgentSocketPath),
		"export PROMPTLOCK_OPERATOR_UNIX_SOCKET=" + shellQuote(layout.OperatorSocketPath),
		"export PROMPTLOCK_OPERATOR_TOKEN=" + shellQuote(operatorToken),
		"export PROMPTLOCK_AUTH_STORE_KEY=" + shellQuote(authKey),
		"export " + secretEnvName + "=" + shellQuote(demoSecretValue),
	}
	if strings.TrimSpace(layout.AgentBridgeAddress) != "" {
		lines = append(lines,
			"export PROMPTLOCK_AGENT_BRIDGE_ADDRESS="+shellQuote(layout.AgentBridgeAddress),
			`export PROMPTLOCK_DOCKER_HOST_ALIAS="${PROMPTLOCK_DOCKER_HOST_ALIAS:-host.docker.internal}"`,
		)
		if bridgeURL := dockerBridgeURLExpression(layout.AgentBridgeAddress); bridgeURL != "" {
			lines = append(lines, `export PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL="`+bridgeURL+`"`)
		}
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func newWorkspaceSetupResult(layout workspaceSetupLayout, opts workspaceSetupOptions, created bool) workspaceSetupResult {
	workspaceID := layout.WorkspaceID
	return workspaceSetupResult{
		Created:            created,
		WorkspaceRoot:      layout.WorkspaceRoot,
		InstanceDir:        layout.InstanceDir,
		SocketDir:          layout.SocketDir,
		ConfigPath:         layout.ConfigPath,
		EnvPath:            layout.EnvPath,
		AuditPath:          layout.AuditPath,
		StateStorePath:     layout.StateStorePath,
		AuthStorePath:      layout.AuthStorePath,
		AgentSocketPath:    layout.AgentSocketPath,
		OperatorSocketPath: layout.OperatorSocketPath,
		AgentBridgeAddress: layout.AgentBridgeAddress,
		DockerBridgeURL:    dockerBridgeURL(layout.AgentBridgeAddress),
		IntentName:         defaultIfEmpty(opts.IntentName, defaultSetupIntentName),
		SecretName:         defaultIfEmpty(opts.SecretName, defaultSetupSecretName),
		OutputSecurityMode: defaultIfEmpty(opts.OutputSecurityMode, defaultSetupOutputSecurityMode),
		AgentID:            workspaceID + "-agent",
		ContainerID:        workspaceID + "-container-1",
		TaskID:             workspaceID + "-quickstart",
		ImageName:          "promptlock-agent-lab",
	}
}

func renderWorkspaceSetupSummary(result workspaceSetupResult) string {
	sourceCmd := ". " + shellQuote(result.EnvPath)
	changeDirCmd := "cd " + shellQuote(result.WorkspaceRoot)
	creationVerb := "ready"
	if !result.Created {
		creationVerb = "reused"
	}
	lines := []string{
		"PromptLock local docker quickstart is " + creationVerb + ".",
		"You are ready for the first approval flow in this workspace.",
		"",
		"Workspace root: " + result.WorkspaceRoot,
		"Instance dir:   " + result.InstanceDir,
		"Socket dir:     " + result.SocketDir,
		"Config file:    " + result.ConfigPath,
		"Env file:       " + result.EnvPath,
		"Audit log:      " + result.AuditPath,
		"",
		"Next commands:",
		"Run the following commands exactly once in three terminals:",
		"Terminal A (broker host):",
		"  " + changeDirCmd,
		"  " + sourceCmd,
		"  go run ./cmd/promptlock daemon start",
		"Terminal B (operator watch UI):",
		"  " + changeDirCmd,
		"  " + sourceCmd,
		"  go run ./cmd/promptlock watch",
		"Terminal C (agent container launch):",
		"  " + changeDirCmd,
		"  docker build -t " + result.ImageName + " .",
		"  " + sourceCmd,
		"  go run ./cmd/promptlock auth docker-run \\",
		"    --agent " + result.AgentID + " \\",
		"    --container " + result.ContainerID + " \\",
		"    --image " + result.ImageName + " \\",
		"    --entrypoint /usr/local/bin/promptlock \\",
		"    -- \\",
		"    exec \\",
		"    --agent " + result.AgentID + " \\",
		"    --task " + result.TaskID + " \\",
		"    --intent " + result.IntentName + " \\",
		"    --reason " + shellQuote("workspace quickstart") + " \\",
		"    --ttl 20 \\",
		"    --wait-approve 5m \\",
		"    --poll-interval 2s \\",
		"    --broker-exec \\",
		"    -- go version",
		"",
		"Notes:",
		"  - The generated config and runtime env live outside the repo so supported state does not sit in the agent-controlled workspace.",
		"  - Quickstart socket paths live under " + result.SocketDir + " so the local Unix-socket flow stays below desktop path-length limits.",
		"  - The env file includes a demo " + setupSecretEnvName(result.SecretName) + " value for local quickstart only. Replace it before real use.",
		"  - The generated quickstart config sets execution output to " + result.OutputSecurityMode + " so the first broker-exec demo prints output. Use output_security_mode=none for stronger containment once the flow is verified.",
	}
	if result.DockerBridgeURL != "" {
		lines = append(lines, "  - On non-Linux desktop Docker runtimes, the daemon also starts an agent-only loopback bridge for containerized MCP clients at "+result.DockerBridgeURL+".")
	} else if strings.TrimSpace(result.AgentBridgeAddress) != "" {
		lines = append(lines, "  - On non-Linux desktop Docker runtimes, the daemon also starts an agent-only loopback bridge for containerized MCP clients.")
		lines = append(lines, "  - After daemon start, use `go run ./cmd/promptlock daemon status --json` to discover the active container bridge URL.")
	}
	return strings.Join(lines, "\n") + "\n"
}

func ensureWorkspaceSocketDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("workspace socket dir is required")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create workspace socket dir %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("chmod workspace socket dir %s: %w", path, err)
	}
	return nil
}

func refreshExistingWorkspaceSetupPaths(layout workspaceSetupLayout) (bool, error) {
	configChanged, err := refreshWorkspaceSetupConfigPaths(layout)
	if err != nil {
		return false, err
	}
	envChanged, err := refreshWorkspaceSetupEnvPaths(layout)
	if err != nil {
		return false, err
	}
	return configChanged || envChanged, nil
}

func refreshWorkspaceSetupConfigPaths(layout workspaceSetupLayout) (bool, error) {
	b, err := os.ReadFile(layout.ConfigPath)
	if err != nil {
		return false, fmt.Errorf("read existing setup config %s: %w", layout.ConfigPath, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return false, fmt.Errorf("decode existing setup config %s: %w", layout.ConfigPath, err)
	}

	changed := false
	changed = setMapStringValue(doc, "agent_unix_socket", layout.AgentSocketPath) || changed
	changed = setMapStringValue(doc, "operator_unix_socket", layout.OperatorSocketPath) || changed
	if strings.TrimSpace(layout.AgentBridgeAddress) != "" {
		changed = setMapStringValue(doc, "agent_bridge_address", layout.AgentBridgeAddress) || changed
	} else if _, ok := doc["agent_bridge_address"]; ok {
		delete(doc, "agent_bridge_address")
		changed = true
	}
	if !changed {
		return false, nil
	}

	updated, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal refreshed setup config %s: %w", layout.ConfigPath, err)
	}
	if err := writePrivateFile(layout.ConfigPath, append(updated, '\n')); err != nil {
		return false, fmt.Errorf("write refreshed setup config %s: %w", layout.ConfigPath, err)
	}
	return true, nil
}

func refreshWorkspaceSetupEnvPaths(layout workspaceSetupLayout) (bool, error) {
	b, err := os.ReadFile(layout.EnvPath)
	if err != nil {
		return false, fmt.Errorf("read existing setup env file %s: %w", layout.EnvPath, err)
	}
	lines := strings.Split(string(b), "\n")
	exports := desiredWorkspaceSetupPathExports(layout)
	updatedLines, changed := syncSetupExportLines(lines, exports)
	if !changed {
		return false, nil
	}
	if err := writePrivateFile(layout.EnvPath, []byte(strings.Join(updatedLines, "\n"))); err != nil {
		return false, fmt.Errorf("write refreshed setup env file %s: %w", layout.EnvPath, err)
	}
	return true, nil
}

type setupExportLine struct {
	Name  string
	Value string
}

func desiredWorkspaceSetupPathExports(layout workspaceSetupLayout) []setupExportLine {
	exports := []setupExportLine{
		{Name: "PROMPTLOCK_SETUP_SOCKET_DIR", Value: shellQuote(layout.SocketDir)},
		{Name: "PROMPTLOCK_CONFIG", Value: shellQuote(layout.ConfigPath)},
		{Name: "PROMPTLOCK_AGENT_UNIX_SOCKET", Value: shellQuote(layout.AgentSocketPath)},
		{Name: "PROMPTLOCK_OPERATOR_UNIX_SOCKET", Value: shellQuote(layout.OperatorSocketPath)},
	}
	if strings.TrimSpace(layout.AgentBridgeAddress) != "" {
		exports = append(exports,
			setupExportLine{Name: "PROMPTLOCK_AGENT_BRIDGE_ADDRESS", Value: shellQuote(layout.AgentBridgeAddress)},
			setupExportLine{Name: "PROMPTLOCK_DOCKER_HOST_ALIAS", Value: `"${PROMPTLOCK_DOCKER_HOST_ALIAS:-host.docker.internal}"`},
		)
		if bridgeURL := dockerBridgeURLExpression(layout.AgentBridgeAddress); bridgeURL != "" {
			exports = append(exports, setupExportLine{Name: "PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL", Value: `"` + bridgeURL + `"`})
		}
	}
	return exports
}

func syncSetupExportLines(lines []string, exports []setupExportLine) ([]string, bool) {
	if len(lines) == 0 {
		lines = []string{""}
	}
	changed := false
	desiredByName := map[string]setupExportLine{}
	for _, export := range exports {
		desiredByName[export.Name] = export
	}
	managedNames := map[string]struct{}{
		"PROMPTLOCK_SETUP_SOCKET_DIR":        {},
		"PROMPTLOCK_CONFIG":                  {},
		"PROMPTLOCK_AGENT_UNIX_SOCKET":       {},
		"PROMPTLOCK_OPERATOR_UNIX_SOCKET":    {},
		"PROMPTLOCK_AGENT_BRIDGE_ADDRESS":    {},
		"PROMPTLOCK_DOCKER_HOST_ALIAS":       {},
		"PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL": {},
	}
	indexByName := map[string]int{}
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name, ok := setupExportName(trimmed); ok {
			if _, managed := managedNames[name]; managed {
				export, wanted := desiredByName[name]
				if !wanted {
					changed = true
					continue
				}
				want := "export " + export.Name + "=" + export.Value
				indexByName[export.Name] = len(filtered)
				if line != want {
					filtered = append(filtered, want)
					changed = true
				} else {
					filtered = append(filtered, line)
				}
				continue
			}
		}
		filtered = append(filtered, line)
	}
	lines = filtered
	insertAt := len(lines)
	if insertAt > 0 && lines[insertAt-1] == "" {
		insertAt--
	}
	for _, export := range exports {
		if _, ok := indexByName[export.Name]; ok {
			continue
		}
		want := "export " + export.Name + "=" + export.Value
		lines = append(lines[:insertAt], append([]string{want}, lines[insertAt:]...)...)
		insertAt++
		changed = true
	}
	return lines, changed
}

func setupExportName(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "export ") {
		return "", false
	}
	raw := strings.TrimPrefix(trimmed, "export ")
	name, _, ok := strings.Cut(raw, "=")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(name), true
}

func setMapStringValue(doc map[string]any, key, want string) bool {
	if doc == nil {
		return false
	}
	if got, _ := doc[key].(string); got == want {
		return false
	}
	doc[key] = want
	return true
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func mustPort(address string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return ""
	}
	return port
}

func dockerBridgeURL(address string) string {
	port := mustPort(address)
	if port == "" || port == "0" {
		return ""
	}
	return "http://host.docker.internal:" + port
}

func dockerBridgeURLExpression(address string) string {
	port := mustPort(address)
	if port == "" || port == "0" {
		return ""
	}
	return "http://${PROMPTLOCK_DOCKER_HOST_ALIAS}:" + port
}

func setupSecretEnvName(secretName string) string {
	return "PROMPTLOCK_SECRET_" + strings.ToUpper(strings.TrimSpace(secretName))
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func writePrivateFile(path string, body []byte) error {
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
