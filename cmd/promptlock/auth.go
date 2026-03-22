package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/lunemec/promptlock/internal/mcplaunchenv"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func runAuth(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, authHelpText())
		os.Exit(2)
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Print(authHelpText())
	case "bootstrap":
		runAuthBootstrap(args[1:])
	case "pair":
		runAuthPair(args[1:])
	case "mint":
		runAuthMint(args[1:])
	case "login":
		runAuthLogin(args[1:])
	case "docker-run":
		runAuthDockerRun(args[1:])
	case "help":
		fmt.Print(authHelpText())
	default:
		fmt.Fprintf(os.Stderr, "unknown auth subcommand: %s\n", args[0])
		fmt.Fprint(os.Stderr, authHelpText())
		os.Exit(2)
	}
}

type authBootstrapResult struct {
	BootstrapToken string    `json:"bootstrap_token"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type authPairResult struct {
	GrantID           string    `json:"grant_id"`
	IdleExpiresAt     time.Time `json:"idle_expires_at"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at"`
}

type authMintResult struct {
	SessionToken string    `json:"session_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type authLoginResult struct {
	SessionToken string    `json:"session_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	GrantID      string    `json:"grant_id"`
}

func authBootstrap(broker, brokerUnix, operatorToken, agentID, containerID string) (authBootstrapResult, error) {
	var out authBootstrapResult
	if err := doPostJSONAuth(broker, brokerUnix, "/v1/auth/bootstrap/create", operatorToken, map[string]string{"agent_id": agentID, "container_id": containerID}, &out); err != nil {
		return authBootstrapResult{}, err
	}
	if strings.TrimSpace(out.BootstrapToken) == "" {
		return authBootstrapResult{}, fmt.Errorf("empty bootstrap_token")
	}
	return out, nil
}

func authPair(broker, brokerUnix, token, containerID string) (authPairResult, error) {
	var out authPairResult
	if err := doPostJSONAuth(broker, brokerUnix, "/v1/auth/pair/complete", "", map[string]string{"token": token, "container_id": containerID}, &out); err != nil {
		return authPairResult{}, err
	}
	if strings.TrimSpace(out.GrantID) == "" {
		return authPairResult{}, fmt.Errorf("empty grant_id")
	}
	return out, nil
}

func authMint(broker, brokerUnix, grantID string) (authMintResult, error) {
	var out authMintResult
	if err := doPostJSONAuth(broker, brokerUnix, "/v1/auth/session/mint", "", map[string]string{"grant_id": grantID}, &out); err != nil {
		return authMintResult{}, err
	}
	if strings.TrimSpace(out.SessionToken) == "" {
		return authMintResult{}, fmt.Errorf("empty session_token")
	}
	return out, nil
}

func authLogin(broker, brokerUnix, operatorToken, agentID, containerID string) (authLoginResult, error) {
	operatorBroker, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{BaseURL: broker, UnixSocket: brokerUnix})
	if err != nil {
		return authLoginResult{}, err
	}
	agentBroker, err := resolveBrokerSelection(brokerRoleAgent, brokerSelectionInput{BaseURL: broker, UnixSocket: brokerUnix})
	if err != nil {
		return authLoginResult{}, err
	}
	bootstrap, err := authBootstrap(operatorBroker.BaseURL, operatorBroker.UnixSocket, operatorToken, agentID, containerID)
	if err != nil {
		return authLoginResult{}, fmt.Errorf("bootstrap step failed: %w", err)
	}
	pair, err := authPair(agentBroker.BaseURL, agentBroker.UnixSocket, bootstrap.BootstrapToken, containerID)
	if err != nil {
		return authLoginResult{}, fmt.Errorf("pair step failed: %w", err)
	}
	mint, err := authMint(agentBroker.BaseURL, agentBroker.UnixSocket, pair.GrantID)
	if err != nil {
		return authLoginResult{}, fmt.Errorf("mint step failed: %w", err)
	}
	return authLoginResult{
		SessionToken: mint.SessionToken,
		ExpiresAt:    mint.ExpiresAt,
		GrantID:      pair.GrantID,
	}, nil
}

func runAuthBootstrap(args []string) {
	fs := flag.NewFlagSet("auth bootstrap", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	opToken := fs.String("operator-token", defaultOperatorToken(), "operator token")
	agent := fs.String("agent", "", "agent id")
	container := fs.String("container", "", "container id")
	fs.Parse(args)
	if strings.TrimSpace(*opToken) == "" || strings.TrimSpace(*agent) == "" || strings.TrimSpace(*container) == "" {
		fatal(fmt.Errorf("--operator-token, --agent and --container are required"))
	}
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	out, err := authBootstrap(broker.BaseURL, broker.UnixSocket, *opToken, *agent, *container)
	if err != nil {
		fatal(err)
	}
	writeJSONStdout(map[string]any{"bootstrap_token": out.BootstrapToken, "expires_at": out.ExpiresAt})
}

func runAuthPair(args []string) {
	fs := flag.NewFlagSet("auth pair", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	token := fs.String("token", "", "bootstrap token")
	container := fs.String("container", "", "container id")
	fs.Parse(args)
	if strings.TrimSpace(*token) == "" || strings.TrimSpace(*container) == "" {
		fatal(fmt.Errorf("--token and --container are required"))
	}
	broker, err := conn.resolve(brokerRoleAgent)
	if err != nil {
		fatal(err)
	}
	out, err := authPair(broker.BaseURL, broker.UnixSocket, *token, *container)
	if err != nil {
		fatal(err)
	}
	writeJSONStdout(map[string]any{"grant_id": out.GrantID, "idle_expires_at": out.IdleExpiresAt, "absolute_expires_at": out.AbsoluteExpiresAt})
}

func runAuthMint(args []string) {
	fs := flag.NewFlagSet("auth mint", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	grant := fs.String("grant", "", "grant id")
	fs.Parse(args)
	if strings.TrimSpace(*grant) == "" {
		fatal(fmt.Errorf("--grant is required"))
	}
	broker, err := conn.resolve(brokerRoleAgent)
	if err != nil {
		fatal(err)
	}
	out, err := authMint(broker.BaseURL, broker.UnixSocket, *grant)
	if err != nil {
		fatal(err)
	}
	writeJSONStdout(map[string]any{"session_token": out.SessionToken, "expires_at": out.ExpiresAt})
}

func runAuthLogin(args []string) {
	fs := flag.NewFlagSet("auth login", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	opToken := fs.String("operator-token", defaultOperatorToken(), "operator token")
	agent := fs.String("agent", "", "agent id")
	container := fs.String("container", "", "container id")
	showGrantID := fs.Bool("show-grant-id", false, "include pairing grant id in stdout output")
	showSecrets := fs.Bool("show-secrets", false, "include raw bearer credentials in stdout output")
	fs.Parse(args)
	if strings.TrimSpace(*opToken) == "" || strings.TrimSpace(*agent) == "" || strings.TrimSpace(*container) == "" {
		fatal(fmt.Errorf("--operator-token, --agent and --container are required"))
	}
	out, err := authLogin(strings.TrimSpace(*conn.Broker), strings.TrimSpace(*conn.BrokerUnix), *opToken, *agent, *container)
	if err != nil {
		fatal(err)
	}
	outJSON := map[string]any{
		"expires_at": out.ExpiresAt,
	}
	if *showSecrets {
		outJSON["session_token"] = out.SessionToken
	}
	if *showGrantID || *showSecrets {
		outJSON["grant_id"] = out.GrantID
	}
	writeJSONStdout(outJSON)
}

type dockerRunConfig struct {
	Image                 string
	ContainerName         string
	WrapperAgentID        string
	WrapperTaskID         string
	SessionToken          string
	BrokerURL             string
	BrokerUnixSocket      string
	ContainerBrokerSocket string
	WrapperMCPEnvFile     string
	ContainerMCPEnvFile   string
	User                  string
	Entrypoint            string
	Workdir               string
	AdditionalMounts      []string
	HiddenPaths           []string
	HiddenMounts          []dockerHiddenMount
	AdditionalEnv         []string
	AdditionalDockerArgs  []string
	Command               []string
	AttachTTY             bool
}

type dockerHiddenMount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

func runAuthDockerRun(args []string) {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("auth docker-run", flag.ContinueOnError)
		registerBrokerFlags(fs)
		fs.String("operator-token", defaultOperatorToken(), "operator token")
		fs.String("agent", "", "agent id")
		fs.String("container", "", "container id / docker name")
		fs.String("image", "", "docker image to run")
		fs.String("container-broker-socket", "/run/promptlock/promptlock-agent.sock", authDockerRunContainerBrokerSocketHelp())
		fs.String("entrypoint", "", "optional docker entrypoint override")
		fs.String("workdir", "", "optional working directory inside container")
		var mounts stringSliceFlag
		var hiddenPaths stringSliceFlag
		var envs stringSliceFlag
		var dockerArgs stringSliceFlag
		fs.Var(&mounts, "mount", "additional docker --mount spec (repeatable)")
		fs.Var(&hiddenPaths, "hide-path", "container path to shadow with an empty readonly placeholder (repeatable); relative paths resolve from --workdir")
		fs.Var(&envs, "env", "additional container env KEY=VALUE (repeatable)")
		fs.Var(&dockerArgs, "docker-arg", "additional allowlisted docker run flag or flag value (repeatable)")
		printFlagHelp(os.Stdout, authDockerRunHelpText(), fs)
		return
	}

	fs := flag.NewFlagSet("auth docker-run", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	opToken := fs.String("operator-token", defaultOperatorToken(), "operator token")
	agent := fs.String("agent", "", "agent id")
	container := fs.String("container", "", "container id / docker name")
	image := fs.String("image", "", "docker image to run")
	containerSocket := fs.String("container-broker-socket", "/run/promptlock/promptlock-agent.sock", authDockerRunContainerBrokerSocketHelp())
	entrypoint := fs.String("entrypoint", "", "optional docker entrypoint override")
	workdir := fs.String("workdir", "", "optional working directory inside container")
	var mounts stringSliceFlag
	var hiddenPaths stringSliceFlag
	var envs stringSliceFlag
	var dockerArgs stringSliceFlag
	fs.Var(&mounts, "mount", "additional docker --mount spec (repeatable)")
	fs.Var(&hiddenPaths, "hide-path", "container path to shadow with an empty readonly placeholder (repeatable); relative paths resolve from --workdir")
	fs.Var(&envs, "env", "additional container env KEY=VALUE (repeatable)")
	fs.Var(&dockerArgs, "docker-arg", "additional allowlisted docker run flag or flag value (repeatable)")
	fs.Parse(args)
	if strings.TrimSpace(*opToken) == "" || strings.TrimSpace(*agent) == "" || strings.TrimSpace(*container) == "" || strings.TrimSpace(*image) == "" {
		fatal(fmt.Errorf("--operator-token, --agent, --container and --image are required"))
	}

	loginResult, err := authLogin(strings.TrimSpace(*conn.Broker), strings.TrimSpace(*conn.BrokerUnix), *opToken, *agent, *container)
	if err != nil {
		fatal(err)
	}
	agentBroker, err := conn.resolve(brokerRoleAgent)
	if err != nil {
		fatal(err)
	}
	brokerTransport, err := resolveDockerBrokerTransport(dockerRuntimeGOOS, agentBroker, *containerSocket)
	if err != nil {
		fatal(err)
	}
	defer func() {
		if err := brokerTransport.Close(); err != nil {
			fatal(err)
		}
	}()

	runCfg := dockerRunConfig{
		Image:                 *image,
		ContainerName:         *container,
		WrapperAgentID:        *agent,
		WrapperTaskID:         *container,
		SessionToken:          loginResult.SessionToken,
		BrokerURL:             brokerTransport.BrokerURL,
		BrokerUnixSocket:      brokerTransport.BrokerUnixSocket,
		ContainerBrokerSocket: brokerTransport.ContainerBrokerSocket,
		ContainerMCPEnvFile:   mcplaunchenv.DefaultFilePath,
		User:                  currentUserDockerIdentity(),
		Entrypoint:            *entrypoint,
		Workdir:               *workdir,
		AdditionalMounts:      mounts,
		HiddenPaths:           hiddenPaths,
		AdditionalEnv:         envs,
		AdditionalDockerArgs:  dockerArgs,
		Command:               fs.Args(),
		AttachTTY:             isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout),
	}
	wrapperMCPEnvFile, err := writeDockerRunWrapperMCPEnvFile(runCfg)
	if err != nil {
		fatal(err)
	}
	runCfg.WrapperMCPEnvFile = wrapperMCPEnvFile
	defer func() {
		_ = os.Remove(wrapperMCPEnvFile)
	}()
	runCfg, hiddenCleanup, err := prepareDockerRunHiddenMounts(runCfg)
	if err != nil {
		fatal(err)
	}
	defer hiddenCleanup()
	runArgs, err := buildDockerRunArgs(runCfg)
	if err != nil {
		fatal(err)
	}

	cmd := exec.Command("docker", runArgs...)
	cmd.Env = buildDockerRunEnv(os.Environ(), runCfg)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if guidance := dockerRunPostLaunchGuidance(runCfg); guidance != "" {
		fmt.Fprintln(os.Stderr, guidance)
		fmt.Fprintln(os.Stderr)
	}
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fatal(err)
	}
}

func authDockerRunHelpText() string {
	return strings.TrimSpace(`
PromptLock auth docker-run

Usage:
  promptlock auth docker-run [flags] -- <container command> [args...]

Run this on the host to mint a short-lived agent session and launch `+"`docker run`"+` in one step.
The wrapper bootstraps on the operator socket, pairs and mints on the agent socket, then wires only agent-side PromptLock transport into the container.
The operator socket stays on the host.
Wrapper-launched containers also export stable wrapper env (`+"`PROMPTLOCK_WRAPPER_AGENT_ID`"+`, `+"`PROMPTLOCK_WRAPPER_TASK_ID`"+`, `+"`PROMPTLOCK_WRAPPER_SESSION_TOKEN`"+`, and current wrapper transport) and now mount a live wrapper MCP env file for `+"`promptlock-mcp-launch`"+` / `+"`promptlock-mcp`"+`, so in-container MCP clients like Codex can keep a portable one-time registration without persisting per-session session-token or transport values.
Interactive Codex-style launches that mount a Codex home also print an in-container reminder to run `+"`promptlock mcp doctor`"+` before starting Codex.

Typical evaluator flow:
  1. Run `+"`promptlock setup`"+` once for the workspace
  2. Start `+"`promptlockd`"+` on the host
  3. Start `+"`promptlock watch`"+` on the host
  4. Run this command from the host to launch the containerized agent

Useful flags:
  - `+"`--mount`"+` passes through workspace or home-directory bind mounts.
  - `+"`--hide-path`"+` shadows mounted files or directories with empty readonly placeholders inside the container. Use it when you keep demo or real `+"`.env`"+` / SOPS files in the repo but do not want the container to read them directly.

Example:
  promptlock auth docker-run \
    --agent toolbelt-agent \
    --container toolbelt-container-1 \
    --image promptlock-agent-lab \
    --entrypoint /usr/local/bin/promptlock \
    -- \
    exec --agent toolbelt-agent --task quickstart --intent run_tests --broker-exec -- go version
`) + "\n"
}

func buildDockerRunArgs(cfg dockerRunConfig) ([]string, error) {
	if strings.TrimSpace(cfg.Image) == "" {
		return nil, fmt.Errorf("docker image is required")
	}
	if strings.TrimSpace(cfg.SessionToken) == "" {
		return nil, fmt.Errorf("session token is required")
	}
	if strings.TrimSpace(cfg.BrokerUnixSocket) == "" && strings.TrimSpace(cfg.BrokerURL) == "" {
		return nil, fmt.Errorf("either broker URL or broker unix socket is required")
	}
	if err := validateDockerRunSecurity(cfg); err != nil {
		return nil, err
	}

	args := []string{"run", "--rm"}
	if cfg.AttachTTY {
		args = append(args, "-it")
	}
	if strings.TrimSpace(cfg.ContainerName) != "" {
		args = append(args, "--name", cfg.ContainerName)
	}
	if user := strings.TrimSpace(cfg.User); user != "" {
		args = append(args, "--user", user)
	}
	args = append(args,
		"--read-only",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "256",
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=64m",
	)
	if strings.TrimSpace(cfg.WrapperMCPEnvFile) != "" && strings.TrimSpace(cfg.ContainerMCPEnvFile) != "" {
		args = append(args, "--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,readonly", filepath.Clean(cfg.WrapperMCPEnvFile), cfg.ContainerMCPEnvFile))
	}

	if strings.TrimSpace(cfg.BrokerUnixSocket) != "" {
		containerSocket := strings.TrimSpace(cfg.ContainerBrokerSocket)
		if containerSocket == "" {
			containerSocket = "/run/promptlock/promptlock-agent.sock"
		}
		args = append(args,
			"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s", filepath.Clean(cfg.BrokerUnixSocket), containerSocket),
			"-e", "PROMPTLOCK_AGENT_UNIX_SOCKET="+containerSocket,
			"-e", "PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET",
		)
	} else {
		args = append(args, "-e", "PROMPTLOCK_BROKER_URL", "-e", "PROMPTLOCK_WRAPPER_BROKER_URL")
	}

	args = append(args, "-e", "PROMPTLOCK_SESSION_TOKEN", "-e", "PROMPTLOCK_WRAPPER_SESSION_TOKEN")
	if strings.TrimSpace(cfg.WrapperAgentID) != "" {
		args = append(args, "-e", "PROMPTLOCK_WRAPPER_AGENT_ID")
	}
	if strings.TrimSpace(cfg.WrapperTaskID) != "" {
		args = append(args, "-e", "PROMPTLOCK_WRAPPER_TASK_ID")
	}
	if strings.TrimSpace(cfg.Entrypoint) != "" {
		args = append(args, "--entrypoint", cfg.Entrypoint)
	}
	if strings.TrimSpace(cfg.Workdir) != "" {
		args = append(args, "--workdir", cfg.Workdir)
	}
	for _, mount := range cfg.AdditionalMounts {
		if strings.TrimSpace(mount) == "" {
			continue
		}
		args = append(args, "--mount", mount)
	}
	for _, hiddenMount := range cfg.HiddenMounts {
		source := strings.TrimSpace(hiddenMount.Source)
		destination := cleanDockerContainerPath(hiddenMount.Destination)
		if source == "" || destination == "" {
			continue
		}
		spec := fmt.Sprintf("type=bind,src=%s,dst=%s", filepath.Clean(source), destination)
		if hiddenMount.ReadOnly {
			spec += ",readonly"
		}
		args = append(args, "--mount", spec)
	}
	for _, envVar := range cfg.AdditionalEnv {
		if strings.TrimSpace(envVar) == "" {
			continue
		}
		args = append(args, "-e", envVar)
	}
	for _, dockerArg := range cfg.AdditionalDockerArgs {
		if strings.TrimSpace(dockerArg) == "" {
			continue
		}
		args = append(args, dockerArg)
	}
	args = append(args, cfg.Image)
	args = append(args, cfg.Command...)
	return args, nil
}

func buildDockerRunEnv(base []string, cfg dockerRunConfig) []string {
	env := append([]string{}, base...)
	env = append(env, "PROMPTLOCK_SESSION_TOKEN="+cfg.SessionToken)
	env = append(env, "PROMPTLOCK_WRAPPER_SESSION_TOKEN="+cfg.SessionToken)
	if strings.TrimSpace(cfg.WrapperAgentID) != "" {
		env = append(env, "PROMPTLOCK_WRAPPER_AGENT_ID="+cfg.WrapperAgentID)
	}
	if strings.TrimSpace(cfg.WrapperTaskID) != "" {
		env = append(env, "PROMPTLOCK_WRAPPER_TASK_ID="+cfg.WrapperTaskID)
	}
	if strings.TrimSpace(cfg.BrokerUnixSocket) != "" {
		containerSocket := strings.TrimSpace(cfg.ContainerBrokerSocket)
		if containerSocket == "" {
			containerSocket = "/run/promptlock/promptlock-agent.sock"
		}
		env = append(env, "PROMPTLOCK_AGENT_UNIX_SOCKET="+containerSocket)
		env = append(env, "PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET="+containerSocket)
		return env
	}
	env = append(env, "PROMPTLOCK_BROKER_URL="+cfg.BrokerURL)
	env = append(env, "PROMPTLOCK_WRAPPER_BROKER_URL="+cfg.BrokerURL)
	return env
}

func writeDockerRunWrapperMCPEnvFile(cfg dockerRunConfig) (string, error) {
	values := map[string]string{
		"PROMPTLOCK_SESSION_TOKEN":         cfg.SessionToken,
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN": cfg.SessionToken,
	}
	order := []string{
		"PROMPTLOCK_SESSION_TOKEN",
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN",
	}
	if strings.TrimSpace(cfg.WrapperAgentID) != "" {
		values["PROMPTLOCK_AGENT_ID"] = cfg.WrapperAgentID
		values["PROMPTLOCK_WRAPPER_AGENT_ID"] = cfg.WrapperAgentID
		order = append(order, "PROMPTLOCK_AGENT_ID", "PROMPTLOCK_WRAPPER_AGENT_ID")
	}
	if strings.TrimSpace(cfg.WrapperTaskID) != "" {
		values["PROMPTLOCK_TASK_ID"] = cfg.WrapperTaskID
		values["PROMPTLOCK_WRAPPER_TASK_ID"] = cfg.WrapperTaskID
		order = append(order, "PROMPTLOCK_TASK_ID", "PROMPTLOCK_WRAPPER_TASK_ID")
	}
	if strings.TrimSpace(cfg.BrokerUnixSocket) != "" {
		containerSocket := strings.TrimSpace(cfg.ContainerBrokerSocket)
		if containerSocket == "" {
			containerSocket = "/run/promptlock/promptlock-agent.sock"
		}
		values["PROMPTLOCK_AGENT_UNIX_SOCKET"] = containerSocket
		values["PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET"] = containerSocket
		order = append(order, "PROMPTLOCK_AGENT_UNIX_SOCKET", "PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET")
	} else if strings.TrimSpace(cfg.BrokerURL) != "" {
		values["PROMPTLOCK_BROKER_URL"] = cfg.BrokerURL
		values["PROMPTLOCK_WRAPPER_BROKER_URL"] = cfg.BrokerURL
		order = append(order, "PROMPTLOCK_BROKER_URL", "PROMPTLOCK_WRAPPER_BROKER_URL")
	}
	body, err := mcplaunchenv.Format(values, order)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "promptlock-mcp-env-*")
	if err != nil {
		return "", fmt.Errorf("create wrapper MCP env file: %w", err)
	}
	path := file.Name()
	if _, err := file.Write(body); err != nil {
		file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write wrapper MCP env file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close wrapper MCP env file: %w", err)
	}
	return path, nil
}

func dockerRunPostLaunchGuidance(cfg dockerRunConfig) string {
	if !dockerRunLooksLikeInteractiveCodex(cfg) {
		return ""
	}
	lines := []string{
		"PromptLock wrapper note for Codex:",
		"  Inside the container, preflight MCP before starting Codex:",
		"    promptlock mcp doctor",
		"  If this Codex home is fresh and missing the shared PromptLock entry, register once:",
		"    codex mcp add promptlock -- promptlock-mcp-launch",
		"  Then start Codex:",
		"    codex -C /workspace --no-alt-screen",
	}
	return strings.Join(lines, "\n")
}

func dockerRunLooksLikeInteractiveCodex(cfg dockerRunConfig) bool {
	if !cfg.AttachTTY {
		return false
	}
	if strings.Contains(strings.ToLower(cfg.Image), "codex") {
		return true
	}
	if tokenListContains(cfg.Command, "codex") || tokenListContains([]string{cfg.Entrypoint}, "codex") {
		return true
	}
	for _, item := range cfg.AdditionalEnv {
		trimmed := strings.TrimSpace(item)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "CODEX_") || strings.Contains(strings.ToLower(trimmed), ".codex") {
			return true
		}
	}
	for _, item := range cfg.AdditionalMounts {
		if strings.Contains(strings.ToLower(item), ".codex") {
			return true
		}
	}
	return false
}

func tokenListContains(items []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}

var protectedDockerEnvVars = map[string]struct{}{
	"PROMPTLOCK_AGENT_UNIX_SOCKET":         {},
	"PROMPTLOCK_BROKER_URL":                {},
	"PROMPTLOCK_SESSION_TOKEN":             {},
	"PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET": {},
	"PROMPTLOCK_WRAPPER_BROKER_URL":        {},
	"PROMPTLOCK_WRAPPER_SESSION_TOKEN":     {},
}

var allowlistedDockerArgHelp = "allowed docker-arg flags: --pull, --init, --label, --label-file, --hostname, --add-host, --dns, --dns-option, --dns-search, --shm-size, --stop-timeout, --tmpfs, --ulimit"

func validateDockerRunSecurity(cfg dockerRunConfig) error {
	if err := validateAdditionalDockerEnvs(cfg.AdditionalEnv); err != nil {
		return err
	}
	if err := validateAdditionalDockerArgs(cfg.AdditionalDockerArgs); err != nil {
		return err
	}
	containerSocket := strings.TrimSpace(cfg.ContainerBrokerSocket)
	if containerSocket == "" {
		containerSocket = "/run/promptlock/promptlock-agent.sock"
	}
	if err := validateAdditionalDockerMounts(cfg.AdditionalMounts, containerSocket, strings.TrimSpace(cfg.ContainerMCPEnvFile)); err != nil {
		return err
	}
	return validateHiddenDockerMounts(cfg.HiddenMounts, containerSocket, strings.TrimSpace(cfg.ContainerMCPEnvFile))
}

func validateAdditionalDockerEnvs(items []string) error {
	for _, item := range items {
		name := dockerEnvName(item)
		if name == "" {
			return fmt.Errorf("docker --env entries must use KEY=VALUE or KEY syntax, for example --env CODEX_HOME=/workspace/.codex")
		}
		if _, blocked := protectedDockerEnvVars[name]; blocked {
			return fmt.Errorf("docker --env may not override reserved PromptLock variable %s; choose a different variable name and let PromptLock manage session transport env vars", name)
		}
	}
	return nil
}

type dockerArgRule struct {
	expectsValue bool
}

var forbiddenDockerArgs = map[string]string{
	"-e":           "docker-arg may not set environment variables; use --env KEY=VALUE and avoid reserved PromptLock transport variables",
	"--env":        "docker-arg may not set environment variables; use --env KEY=VALUE and avoid reserved PromptLock transport variables",
	"--env-file":   "docker-arg may not use --env-file because it can override PromptLock session transport variables; pass explicit --env entries instead",
	"--mount":      "docker-arg may not add mounts directly; use --mount so PromptLock can validate the protected container broker socket path",
	"-v":           "docker-arg may not add volumes directly; use --mount so PromptLock can validate the protected container broker socket path",
	"--volume":     "docker-arg may not add volumes directly; use --mount so PromptLock can validate the protected container broker socket path",
	"--entrypoint": "docker-arg may not override entrypoint; use the wrapper's --entrypoint flag instead",
	"--workdir":    "docker-arg may not override workdir; use the wrapper's --workdir flag instead",
	"-w":           "docker-arg may not override workdir; use the wrapper's --workdir flag instead",
	"--user":       "docker-arg may not override user; PromptLock manages the container user for auth/session safety",
	"-u":           "docker-arg may not override user; PromptLock manages the container user for auth/session safety",
}

var allowlistedDockerArgs = map[string]dockerArgRule{
	"--add-host":     {expectsValue: true},
	"--dns":          {expectsValue: true},
	"--dns-option":   {expectsValue: true},
	"--dns-search":   {expectsValue: true},
	"-h":             {expectsValue: true},
	"--hostname":     {expectsValue: true},
	"--init":         {},
	"-l":             {expectsValue: true},
	"--label":        {expectsValue: true},
	"--label-file":   {expectsValue: true},
	"--pull":         {expectsValue: true},
	"--shm-size":     {expectsValue: true},
	"--stop-timeout": {expectsValue: true},
	"--tmpfs":        {expectsValue: true},
	"--ulimit":       {expectsValue: true},
}

func validateAdditionalDockerArgs(items []string) error {
	var expectingValue *dockerArgRule
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if expectingValue != nil {
			if strings.HasPrefix(trimmed, "-") {
				return fmt.Errorf("docker-arg missing value before %q; %s", trimmed, allowlistedDockerArgHelp)
			}
			expectingValue = nil
			continue
		}
		flagName, hasInlineValue := parseDockerArgToken(trimmed)
		if flagName == "" {
			return fmt.Errorf("docker-arg %q is not allowed; %s", trimmed, allowlistedDockerArgHelp)
		}
		if msg, blocked := forbiddenDockerArgs[flagName]; blocked {
			return errors.New(msg)
		}
		rule, allowed := allowlistedDockerArgs[flagName]
		if !allowed {
			return fmt.Errorf("docker-arg %q is not allowed; use first-class PromptLock flags like --env, --mount, --workdir, or --entrypoint when available; %s", flagName, allowlistedDockerArgHelp)
		}
		if rule.expectsValue && !hasInlineValue {
			expectingValue = &rule
		}
	}
	if expectingValue != nil {
		return fmt.Errorf("docker-arg missing value for previous flag; %s", allowlistedDockerArgHelp)
	}
	return nil
}

func parseDockerArgToken(item string) (string, bool) {
	trimmed := strings.TrimSpace(item)
	if trimmed == "" || !strings.HasPrefix(trimmed, "-") {
		return "", false
	}
	if strings.HasPrefix(trimmed, "--") {
		if name, _, found := strings.Cut(trimmed, "="); found {
			return name, true
		}
		return trimmed, false
	}
	if strings.HasPrefix(trimmed, "-") && len(trimmed) > 2 {
		if trimmed[2] == '=' {
			return trimmed[:2], true
		}
	}
	return trimmed, false
}

func validateAdditionalDockerMounts(items []string, containerBrokerSocket string, containerMCPEnvFile string) error {
	protectedTargets := protectedDockerMountTargets(containerBrokerSocket, containerMCPEnvFile)
	if len(protectedTargets) == 0 {
		return nil
	}
	for _, item := range items {
		destination := parseDockerMountDestination(item)
		if destination == "" {
			continue
		}
		if protectedName, ok := dockerProtectedTargetOverlap(cleanDockerContainerPath(destination), protectedTargets); ok {
			return fmt.Errorf("additional mount may not target the reserved PromptLock path %s", protectedName)
		}
	}
	return nil
}

func validateHiddenDockerMounts(items []dockerHiddenMount, containerBrokerSocket string, containerMCPEnvFile string) error {
	protectedTargets := protectedDockerMountTargets(containerBrokerSocket, containerMCPEnvFile)
	if len(protectedTargets) == 0 {
		return nil
	}
	for _, item := range items {
		if protectedName, ok := dockerProtectedTargetOverlap(cleanDockerContainerPath(item.Destination), protectedTargets); ok {
			return fmt.Errorf("hidden mount may not target the reserved PromptLock path %s", protectedName)
		}
	}
	return nil
}

func dockerEnvName(item string) string {
	trimmed := strings.TrimSpace(item)
	if trimmed == "" {
		return ""
	}
	name, _, found := strings.Cut(trimmed, "=")
	if !found {
		name = trimmed
	}
	return strings.TrimSpace(name)
}

func parseDockerMountDestination(spec string) string {
	return parseDockerMountSpec(spec).Destination
}

type dockerMountSpec struct {
	Type        string
	Source      string
	Destination string
}

func parseDockerMountSpec(spec string) dockerMountSpec {
	var out dockerMountSpec
	for _, part := range strings.Split(spec, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "type":
			out.Type = strings.TrimSpace(value)
		case "src", "source":
			out.Source = strings.TrimSpace(value)
		case "dst", "destination", "target":
			out.Destination = cleanDockerContainerPath(value)
		}
	}
	return out
}

func cleanDockerContainerPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	return path.Clean(normalized)
}

func dockerContainerPathContains(parent, child string) bool {
	parent = cleanDockerContainerPath(parent)
	child = cleanDockerContainerPath(child)
	if parent == "" || child == "" {
		return false
	}
	return parent == child || strings.HasPrefix(child, parent+"/")
}

func dockerContainerPathsOverlap(left, right string) bool {
	return dockerContainerPathContains(left, right) || dockerContainerPathContains(right, left)
}

func protectedDockerMountTargets(containerBrokerSocket string, containerMCPEnvFile string) map[string]string {
	protectedTargets := map[string]string{}
	if protectedTarget := cleanDockerContainerPath(containerBrokerSocket); protectedTarget != "" {
		protectedTargets[protectedTarget] = fmt.Sprintf("container broker socket %s", protectedTarget)
	}
	if protectedEnvFile := cleanDockerContainerPath(containerMCPEnvFile); protectedEnvFile != "" {
		protectedTargets[protectedEnvFile] = fmt.Sprintf("PromptLock wrapper mcp env file %s", protectedEnvFile)
	}
	return protectedTargets
}

func dockerProtectedTargetOverlap(candidate string, protectedTargets map[string]string) (string, bool) {
	for protectedPath, protectedName := range protectedTargets {
		if dockerContainerPathsOverlap(candidate, protectedPath) {
			return protectedName, true
		}
	}
	return "", false
}

func prepareDockerRunHiddenMounts(cfg dockerRunConfig) (dockerRunConfig, func(), error) {
	if len(cfg.HiddenPaths) == 0 {
		return cfg, func() {}, nil
	}
	protectedTargets := protectedDockerMountTargets(cfg.ContainerBrokerSocket, cfg.ContainerMCPEnvFile)
	bindMounts := dockerRunBindMounts(cfg.AdditionalMounts)
	tempDir, err := os.MkdirTemp("", "promptlock-docker-hide-*")
	if err != nil {
		return cfg, nil, fmt.Errorf("create hide-path temp dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	seenTargets := map[string]struct{}{}
	for idx, rawPath := range cfg.HiddenPaths {
		target, err := resolveDockerHiddenPath(rawPath, cfg.Workdir)
		if err != nil {
			cleanup()
			return cfg, nil, err
		}
		if protectedName, blocked := dockerProtectedTargetOverlap(target, protectedTargets); blocked {
			cleanup()
			return cfg, nil, fmt.Errorf("hide-path %q targets the reserved PromptLock path %s", strings.TrimSpace(rawPath), protectedName)
		}
		if _, exists := seenTargets[target]; exists {
			continue
		}
		maskSource, err := buildDockerHiddenMountSource(tempDir, idx, target, bindMounts)
		if err != nil {
			cleanup()
			return cfg, nil, err
		}
		cfg.HiddenMounts = append(cfg.HiddenMounts, dockerHiddenMount{
			Source:      maskSource,
			Destination: target,
			ReadOnly:    true,
		})
		seenTargets[target] = struct{}{}
	}
	return cfg, cleanup, nil
}

type dockerBindMount struct {
	Source      string
	Destination string
}

func dockerRunBindMounts(items []string) []dockerBindMount {
	out := make([]dockerBindMount, 0, len(items))
	for _, item := range items {
		spec := parseDockerMountSpec(item)
		if !strings.EqualFold(strings.TrimSpace(spec.Type), "bind") {
			continue
		}
		source := strings.TrimSpace(spec.Source)
		destination := cleanDockerContainerPath(spec.Destination)
		if source == "" || destination == "" {
			continue
		}
		out = append(out, dockerBindMount{Source: filepath.Clean(source), Destination: destination})
	}
	return out
}

func resolveDockerHiddenPath(rawPath string, workdir string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", fmt.Errorf("hide-path is required")
	}
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		return "", fmt.Errorf("hide-path %q must be a single-line path", trimmed)
	}
	if path.IsAbs(strings.ReplaceAll(trimmed, "\\", "/")) {
		return cleanDockerContainerPath(trimmed), nil
	}
	if strings.TrimSpace(workdir) == "" {
		return "", fmt.Errorf("relative hide-path %q requires --workdir", trimmed)
	}
	return cleanDockerContainerPath(path.Join(cleanDockerContainerPath(workdir), strings.ReplaceAll(trimmed, "\\", "/"))), nil
}

func buildDockerHiddenMountSource(tempDir string, index int, target string, bindMounts []dockerBindMount) (string, error) {
	hostTarget, isDir, err := dockerHiddenHostTarget(target, bindMounts)
	if err != nil {
		return "", err
	}
	placeholder := filepath.Join(tempDir, fmt.Sprintf("mask-%03d", index))
	if isDir {
		if err := os.MkdirAll(placeholder, 0o755); err != nil {
			return "", fmt.Errorf("create hidden directory placeholder for %q: %w", target, err)
		}
		return placeholder, nil
	}
	if err := os.WriteFile(placeholder, nil, 0o600); err != nil {
		return "", fmt.Errorf("create hidden file placeholder for %q: %w", target, err)
	}
	_ = hostTarget
	return placeholder, nil
}

func dockerHiddenHostTarget(target string, bindMounts []dockerBindMount) (string, bool, error) {
	var best *dockerBindMount
	for i := range bindMounts {
		candidate := bindMounts[i]
		if !dockerContainerPathContains(candidate.Destination, target) {
			continue
		}
		if best == nil || len(candidate.Destination) > len(best.Destination) {
			best = &candidate
		}
	}
	if best == nil {
		return "", false, fmt.Errorf("hide-path %q is not covered by any bind mount; add a matching --mount type=bind,src=...,dst=... first", target)
	}
	hostTarget := best.Source
	if target != best.Destination {
		rel := strings.TrimPrefix(target, best.Destination)
		rel = strings.TrimPrefix(rel, "/")
		hostTarget = filepath.Join(best.Source, filepath.FromSlash(rel))
	}
	info, err := os.Stat(hostTarget)
	if err != nil {
		return "", false, fmt.Errorf("hide-path %q maps to host path %q: %w", target, hostTarget, err)
	}
	return hostTarget, info.IsDir(), nil
}

func currentUserDockerIdentity() string {
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
}
