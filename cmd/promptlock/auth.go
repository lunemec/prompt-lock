package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
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
	opToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
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
	opToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
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
	SessionToken          string
	BrokerURL             string
	BrokerUnixSocket      string
	ContainerBrokerSocket string
	User                  string
	Entrypoint            string
	Workdir               string
	AdditionalMounts      []string
	AdditionalEnv         []string
	AdditionalDockerArgs  []string
	Command               []string
	AttachTTY             bool
}

func runAuthDockerRun(args []string) {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("auth docker-run", flag.ContinueOnError)
		registerBrokerFlags(fs)
		fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
		fs.String("agent", "", "agent id")
		fs.String("container", "", "container id / docker name")
		fs.String("image", "", "docker image to run")
		fs.String("container-broker-socket", "/run/promptlock/promptlock-agent.sock", authDockerRunContainerBrokerSocketHelp())
		fs.String("entrypoint", "", "optional docker entrypoint override")
		fs.String("workdir", "", "optional working directory inside container")
		var mounts stringSliceFlag
		var envs stringSliceFlag
		var dockerArgs stringSliceFlag
		fs.Var(&mounts, "mount", "additional docker --mount spec (repeatable)")
		fs.Var(&envs, "env", "additional container env KEY=VALUE (repeatable)")
		fs.Var(&dockerArgs, "docker-arg", "additional allowlisted docker run flag or flag value (repeatable)")
		printFlagHelp(os.Stdout, authDockerRunHelpText(), fs)
		return
	}

	fs := flag.NewFlagSet("auth docker-run", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	opToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	agent := fs.String("agent", "", "agent id")
	container := fs.String("container", "", "container id / docker name")
	image := fs.String("image", "", "docker image to run")
	containerSocket := fs.String("container-broker-socket", "/run/promptlock/promptlock-agent.sock", authDockerRunContainerBrokerSocketHelp())
	entrypoint := fs.String("entrypoint", "", "optional docker entrypoint override")
	workdir := fs.String("workdir", "", "optional working directory inside container")
	var mounts stringSliceFlag
	var envs stringSliceFlag
	var dockerArgs stringSliceFlag
	fs.Var(&mounts, "mount", "additional docker --mount spec (repeatable)")
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

	runArgs, err := buildDockerRunArgs(dockerRunConfig{
		Image:                 *image,
		ContainerName:         *container,
		SessionToken:          loginResult.SessionToken,
		BrokerURL:             agentBroker.BaseURL,
		BrokerUnixSocket:      agentBroker.UnixSocket,
		ContainerBrokerSocket: *containerSocket,
		User:                  currentUserDockerIdentity(),
		Entrypoint:            *entrypoint,
		Workdir:               *workdir,
		AdditionalMounts:      mounts,
		AdditionalEnv:         envs,
		AdditionalDockerArgs:  dockerArgs,
		Command:               fs.Args(),
		AttachTTY:             isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout),
	})
	if err != nil {
		fatal(err)
	}

	cmd := exec.Command("docker", runArgs...)
	cmd.Env = buildDockerRunEnv(os.Environ(), dockerRunConfig{
		SessionToken:          loginResult.SessionToken,
		BrokerURL:             agentBroker.BaseURL,
		BrokerUnixSocket:      agentBroker.UnixSocket,
		ContainerBrokerSocket: *containerSocket,
	})
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
The wrapper bootstraps on the operator socket, pairs and mints on the agent socket, then mounts only the agent socket into the container.
The operator socket stays on the host.

Typical evaluator flow:
  1. Run `+"`promptlock setup`"+` once for the workspace
  2. Start `+"`promptlockd`"+` on the host
  3. Start `+"`promptlock watch`"+` on the host
  4. Run this command from the host to launch the containerized agent

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

	if strings.TrimSpace(cfg.BrokerUnixSocket) != "" {
		containerSocket := strings.TrimSpace(cfg.ContainerBrokerSocket)
		if containerSocket == "" {
			containerSocket = "/run/promptlock/promptlock-agent.sock"
		}
		args = append(args,
			"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s", filepath.Clean(cfg.BrokerUnixSocket), containerSocket),
			"-e", "PROMPTLOCK_AGENT_UNIX_SOCKET="+containerSocket,
		)
	} else {
		args = append(args, "-e", "PROMPTLOCK_BROKER_URL")
	}

	args = append(args, "-e", "PROMPTLOCK_SESSION_TOKEN")
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
	if strings.TrimSpace(cfg.BrokerUnixSocket) != "" {
		containerSocket := strings.TrimSpace(cfg.ContainerBrokerSocket)
		if containerSocket == "" {
			containerSocket = "/run/promptlock/promptlock-agent.sock"
		}
		env = append(env, "PROMPTLOCK_AGENT_UNIX_SOCKET="+containerSocket)
		return env
	}
	env = append(env, "PROMPTLOCK_BROKER_URL="+cfg.BrokerURL)
	return env
}

var protectedDockerEnvVars = map[string]struct{}{
	"PROMPTLOCK_AGENT_UNIX_SOCKET": {},
	"PROMPTLOCK_BROKER_URL":        {},
	"PROMPTLOCK_SESSION_TOKEN":     {},
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
	return validateAdditionalDockerMounts(cfg.AdditionalMounts, containerSocket)
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

func validateAdditionalDockerMounts(items []string, containerBrokerSocket string) error {
	protectedTarget := filepath.Clean(strings.TrimSpace(containerBrokerSocket))
	if protectedTarget == "" {
		return nil
	}
	for _, item := range items {
		destination := parseDockerMountDestination(item)
		if destination == "" {
			continue
		}
		if filepath.Clean(destination) == protectedTarget {
			return fmt.Errorf("additional mount may not target the container broker socket %s", protectedTarget)
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
	for _, part := range strings.Split(spec, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "dst", "destination", "target":
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func currentUserDockerIdentity() string {
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
}
