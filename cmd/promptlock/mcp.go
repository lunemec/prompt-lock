package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type mcpDoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type mcpDoctorReport struct {
	Context         string           `json:"context"`
	CodexConfigPath string           `json:"codex_config_path,omitempty"`
	Ready           bool             `json:"ready"`
	Checks          []mcpDoctorCheck `json:"checks"`
	Hints           []string         `json:"hints,omitempty"`
}

type mcpDoctorDeps struct {
	Env             []string
	Getenv          func(string) string
	LookPath        func(string) (string, error)
	ReadFile        func(string) ([]byte, error)
	FileExists      func(string) bool
	ProbeLauncher   func(context.Context, string, []string) error
	DetectWorkspace func() (workspaceSetupLayout, bool)
}

var (
	mcpDoctorLookPath        = exec.LookPath
	mcpDoctorReadFile        = os.ReadFile
	mcpDoctorFileExists      = fileExists
	mcpDoctorProbeLauncher   = probeMCPLauncher
	mcpDoctorDetectWorkspace = detectWorkspaceSetupLayout
)

func runMCP(args []string) {
	if len(args) == 0 || hasHelpFlag(args) || args[0] == "help" {
		fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
		fs.Bool("json", false, "print structured JSON output (doctor)")
		printFlagHelp(os.Stdout, mcpHelpText(), fs)
		return
	}

	switch strings.TrimSpace(args[0]) {
	case "doctor":
		runMCPDoctor(args[1:])
	default:
		fatal(fmt.Errorf("unknown mcp command: %s", strings.TrimSpace(args[0])))
	}
}

func runMCPDoctor(args []string) {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("mcp doctor", flag.ContinueOnError)
		fs.Bool("json", false, "print structured JSON output")
		printFlagHelp(os.Stdout, mcpDoctorHelpText(), fs)
		return
	}

	fs := flag.NewFlagSet("mcp doctor", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "print structured JSON output")
	_ = fs.Parse(args)

	report := buildMCPDoctorReport(defaultMCPDoctorDeps())
	if *jsonOutput {
		writeJSONStdout(report)
	} else {
		printMCPDoctorReport(report)
	}
	if !report.Ready {
		os.Exit(1)
	}
}

func defaultMCPDoctorDeps() mcpDoctorDeps {
	return mcpDoctorDeps{
		Env:             os.Environ(),
		Getenv:          os.Getenv,
		LookPath:        mcpDoctorLookPath,
		ReadFile:        mcpDoctorReadFile,
		FileExists:      mcpDoctorFileExists,
		ProbeLauncher:   mcpDoctorProbeLauncher,
		DetectWorkspace: mcpDoctorDetectWorkspace,
	}
}

func buildMCPDoctorReport(deps mcpDoctorDeps) mcpDoctorReport {
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.ReadFile == nil {
		deps.ReadFile = os.ReadFile
	}
	if deps.FileExists == nil {
		deps.FileExists = fileExists
	}
	if deps.ProbeLauncher == nil {
		deps.ProbeLauncher = probeMCPLauncher
	}
	if deps.DetectWorkspace == nil {
		deps.DetectWorkspace = detectWorkspaceSetupLayout
	}
	if deps.Env == nil {
		deps.Env = os.Environ()
	}

	report := mcpDoctorReport{
		Context: detectMCPDoctorContext(deps.Getenv),
	}
	if codexPath := codexConfigPath(deps.Getenv); codexPath != "" {
		report.CodexConfigPath = codexPath
	}

	var hasFailure bool
	addCheck := func(name, status, detail string) {
		if status == "fail" {
			hasFailure = true
		}
		report.Checks = append(report.Checks, mcpDoctorCheck{
			Name:   name,
			Status: status,
			Detail: detail,
		})
	}

	launcherPath, err := deps.LookPath("promptlock-mcp-launch")
	if err != nil {
		addCheck("launcher_on_path", "fail", "promptlock-mcp-launch not found on PATH")
		report.Hints = append(report.Hints, "Install the launcher with `go install github.com/lunemec/promptlock/cmd/promptlock-mcp-launch@latest`.")
	} else {
		addCheck("launcher_on_path", "pass", "promptlock-mcp-launch resolves to "+launcherPath)
	}

	if launcherPath != "" {
		mcpPath, resolveErr := resolveMCPDoctorTargetBinary(launcherPath, deps.Getenv, deps.LookPath, deps.FileExists)
		if resolveErr != nil {
			addCheck("mcp_binary_resolution", "fail", resolveErr.Error())
			report.Hints = append(report.Hints, "Install the adapter with `go install github.com/lunemec/promptlock/cmd/promptlock-mcp@latest` or place it next to promptlock-mcp-launch.")
		} else {
			addCheck("mcp_binary_resolution", "pass", "launcher resolves promptlock-mcp at "+mcpPath)
		}
	}

	codexInstalled := false
	if _, err := deps.LookPath("codex"); err == nil {
		codexInstalled = true
	}
	codexPath := codexConfigPath(deps.Getenv)
	codexEntryChecked := false
	if strings.TrimSpace(codexPath) != "" && deps.FileExists(codexPath) {
		command, found, parseErr := readCodexPromptlockCommand(codexPath, deps.ReadFile)
		codexEntryChecked = true
		switch {
		case parseErr != nil:
			addCheck("codex_promptlock_entry", "fail", fmt.Sprintf("failed to inspect %s: %v", codexPath, parseErr))
		case !found:
			addCheck("codex_promptlock_entry", "fail", fmt.Sprintf("Codex config at %s is missing [mcp_servers.promptlock]", codexPath))
			report.Hints = append(report.Hints, "Register the launcher with `codex mcp add promptlock -- promptlock-mcp-launch`.")
		case strings.TrimSpace(command) == "promptlock-mcp-launch":
			addCheck("codex_promptlock_entry", "pass", fmt.Sprintf("Codex config uses portable command %q", command))
		case strings.Contains(command, "/") && filepath.Base(command) == "promptlock-mcp-launch":
			addCheck("codex_promptlock_entry", "fail", fmt.Sprintf("Codex config uses non-portable absolute path %q; prefer command name promptlock-mcp-launch", command))
			report.Hints = append(report.Hints, "Rewrite the Codex entry to use `codex mcp add promptlock -- promptlock-mcp-launch` so it works on both host and in wrapper-launched containers.")
		default:
			addCheck("codex_promptlock_entry", "fail", fmt.Sprintf("Codex config uses unexpected command %q for promptlock", command))
			report.Hints = append(report.Hints, "Register the PromptLock launcher with `codex mcp add promptlock -- promptlock-mcp-launch`.")
		}
	}
	if !codexEntryChecked {
		switch {
		case codexInstalled:
			addCheck("codex_promptlock_entry", "warn", "Codex is installed but no shared Codex config entry was found for promptlock")
			report.Hints = append(report.Hints, "If you use Codex, register PromptLock with `codex mcp add promptlock -- promptlock-mcp-launch`.")
		case strings.TrimSpace(codexPath) != "":
			addCheck("codex_promptlock_entry", "skip", "No Codex config file found; skipping Codex MCP registration check")
		default:
			addCheck("codex_promptlock_entry", "skip", "Codex is not installed in PATH; skipping Codex MCP registration check")
		}
	}

	sessionToken := effectiveMCPDoctorEnv(deps.Getenv, "PROMPTLOCK_SESSION_TOKEN")
	if sessionToken == "" {
		addCheck("session_env", "fail", "no live PROMPTLOCK_SESSION_TOKEN is available in the current shell")
		report.Hints = append(report.Hints, "Launch the MCP client from a shell with a fresh PromptLock session token, or run it inside `promptlock auth docker-run`.")
	} else {
		addCheck("session_env", "pass", fmt.Sprintf("live session token detected (%d bytes)", len(sessionToken)))
	}

	transportKind, transportValue := mcpDoctorTransport(deps.Getenv)
	if transportValue == "" {
		addCheck("transport_env", "fail", "no live agent transport is available; expected PROMPTLOCK_AGENT_UNIX_SOCKET or PROMPTLOCK_BROKER_URL")
		hint := "Export PROMPTLOCK_AGENT_UNIX_SOCKET or PROMPTLOCK_BROKER_URL before launching the MCP client, or use `promptlock auth docker-run`."
		if layout, ok := deps.DetectWorkspace(); ok {
			if agentSocket := workspaceSetupBrokerSocket(brokerRoleAgent, layout.ConfigPath); strings.TrimSpace(agentSocket) != "" {
				hint = fmt.Sprintf("Current workspace setup provides agent socket %s. Export PROMPTLOCK_AGENT_UNIX_SOCKET=%s before launching the MCP client, or use `promptlock auth docker-run`.", agentSocket, agentSocket)
			}
		}
		report.Hints = append(report.Hints, hint)
	} else {
		addCheck("transport_env", "pass", fmt.Sprintf("live %s transport detected at %s", transportKind, transportValue))
	}

	if launcherPath != "" && sessionToken != "" && transportValue != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := deps.ProbeLauncher(ctx, launcherPath, deps.Env); err != nil {
			addCheck("launcher_handshake", "fail", err.Error())
		} else {
			addCheck("launcher_handshake", "pass", "promptlock-mcp-launch completed initialize/tools/list successfully")
		}
	} else {
		addCheck("launcher_handshake", "skip", "skipped because launcher path, live session, or live transport is missing")
	}

	report.Ready = !hasFailure
	return report
}

func printMCPDoctorReport(report mcpDoctorReport) {
	fmt.Println("PromptLock MCP Doctor")
	fmt.Printf("context: %s\n", report.Context)
	if strings.TrimSpace(report.CodexConfigPath) != "" {
		fmt.Printf("codex config: %s\n", report.CodexConfigPath)
	}
	fmt.Println()
	for _, check := range report.Checks {
		fmt.Printf("%s %s: %s\n", strings.ToUpper(check.Status), check.Name, check.Detail)
	}
	if len(report.Hints) > 0 {
		fmt.Println()
		fmt.Println("Hints:")
		for _, hint := range report.Hints {
			fmt.Printf("- %s\n", hint)
		}
	}
	fmt.Println()
	fmt.Printf("ready: %t\n", report.Ready)
}

func detectMCPDoctorContext(getenv func(string) string) string {
	if strings.TrimSpace(getenv("PROMPTLOCK_WRAPPER_SESSION_TOKEN")) != "" || strings.TrimSpace(getenv("PROMPTLOCK_WRAPPER_AGENT_ID")) != "" {
		return "wrapper_container"
	}
	if effectiveMCPDoctorEnv(getenv, "PROMPTLOCK_SESSION_TOKEN") != "" && effectiveMCPDoctorEnv(getenv, "PROMPTLOCK_AGENT_UNIX_SOCKET") != "" {
		return "shell_with_agent_socket"
	}
	if effectiveMCPDoctorEnv(getenv, "PROMPTLOCK_SESSION_TOKEN") != "" && effectiveMCPDoctorEnv(getenv, "PROMPTLOCK_BROKER_URL") != "" {
		return "shell_with_broker_url"
	}
	return "shell_without_live_promptlock_env"
}

func effectiveMCPDoctorEnv(getenv func(string) string, key string) string {
	switch key {
	case "PROMPTLOCK_SESSION_TOKEN":
		if v := strings.TrimSpace(getenv("PROMPTLOCK_WRAPPER_SESSION_TOKEN")); v != "" {
			return v
		}
	case "PROMPTLOCK_AGENT_UNIX_SOCKET":
		if v := strings.TrimSpace(getenv("PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET")); v != "" {
			return v
		}
	case "PROMPTLOCK_BROKER_URL":
		if v := strings.TrimSpace(getenv("PROMPTLOCK_WRAPPER_BROKER_URL")); v != "" {
			return v
		}
	}
	return strings.TrimSpace(getenv(key))
}

func mcpDoctorTransport(getenv func(string) string) (string, string) {
	if agentUnix := effectiveMCPDoctorEnv(getenv, "PROMPTLOCK_AGENT_UNIX_SOCKET"); agentUnix != "" {
		return "agent unix socket", agentUnix
	}
	if brokerURL := effectiveMCPDoctorEnv(getenv, "PROMPTLOCK_BROKER_URL"); brokerURL != "" {
		return "broker URL", brokerURL
	}
	return "", ""
}

func codexConfigPath(getenv func(string) string) string {
	if codexHome := strings.TrimSpace(getenv("CODEX_HOME")); codexHome != "" {
		return filepath.Join(codexHome, "config.toml")
	}
	if home := strings.TrimSpace(getenv("HOME")); home != "" {
		return filepath.Join(home, ".codex", "config.toml")
	}
	return ""
}

func readCodexPromptlockCommand(path string, readFile func(string) ([]byte, error)) (string, bool, error) {
	body, err := readFile(path)
	if err != nil {
		return "", false, err
	}
	section := ""
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(stripInlineComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if section != "mcp_servers.promptlock" {
			continue
		}
		key, rawValue, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != "command" {
			continue
		}
		value, err := parseSimpleTOMLValue(rawValue)
		if err != nil {
			return "", false, err
		}
		return value, true, nil
	}
	return "", false, nil
}

func stripInlineComment(line string) string {
	inSingle := false
	inDouble := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func parseSimpleTOMLValue(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return "", nil
	case strings.HasPrefix(trimmed, `"`):
		quoted := trimmed
		if !strings.HasSuffix(quoted, `"`) {
			return "", fmt.Errorf("unterminated TOML string %q", trimmed)
		}
		value, err := strconv.Unquote(quoted)
		if err != nil {
			return "", err
		}
		return value, nil
	case strings.HasPrefix(trimmed, `'`) && strings.HasSuffix(trimmed, `'`):
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, `'`), `'`), nil
	default:
		return trimmed, nil
	}
}

func resolveMCPDoctorTargetBinary(launcherPath string, getenv func(string) string, lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	if override := strings.TrimSpace(getenv("PROMPTLOCK_MCP_BIN")); override != "" {
		if exists(override) {
			return override, nil
		}
		return "", fmt.Errorf("PROMPTLOCK_MCP_BIN points to missing executable %s", override)
	}
	sibling := filepath.Join(filepath.Dir(launcherPath), "promptlock-mcp")
	if exists(sibling) {
		return sibling, nil
	}
	if lookPath != nil {
		if target, err := lookPath("promptlock-mcp"); err == nil {
			return target, nil
		}
	}
	return "", errors.New("promptlock-mcp launcher could not find promptlock-mcp")
}

func probeMCPLauncher(ctx context.Context, launcherPath string, env []string) error {
	stdin := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n",
	)
	cmd := exec.CommandContext(ctx, launcherPath)
	cmd.Env = env
	cmd.Stdin = stdin

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("launcher probe failed: %s", msg)
	}

	lines := nonEmptyLines(stdout.String())
	if len(lines) < 2 {
		return fmt.Errorf("launcher probe returned %d stdout lines, want at least 2", len(lines))
	}
	for i, line := range lines[:2] {
		var resp struct {
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return fmt.Errorf("launcher probe response %d is not valid JSON: %w", i+1, err)
		}
		if resp.Error != nil {
			return fmt.Errorf("launcher probe response %d returned MCP error: %v", i+1, resp.Error)
		}
	}
	return nil
}

func nonEmptyLines(body string) []string {
	lines := []string{}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}
