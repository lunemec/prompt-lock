package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/app"
)

type requestBody struct {
	AgentID            string   `json:"agent_id"`
	TaskID             string   `json:"task_id"`
	Intent             string   `json:"intent,omitempty"`
	Reason             string   `json:"reason"`
	TTLMinutes         int      `json:"ttl_minutes"`
	Secrets            []string `json:"secrets"`
	CommandFingerprint string   `json:"command_fingerprint"`
	WorkdirFingerprint string   `json:"workdir_fingerprint"`
	CommandSummary     string   `json:"command_summary,omitempty"`
	WorkdirSummary     string   `json:"workdir_summary,omitempty"`
	EnvPath            string   `json:"env_path,omitempty"`
}

func runExec(args []string) {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("exec", flag.ContinueOnError)
		registerBrokerFlags(fs)
		fs.String("agent", "agent", "agent id")
		fs.String("task", "task", "task id")
		fs.String("session-token", getenv("PROMPTLOCK_SESSION_TOKEN", ""), "agent session token")
		fs.String("reason", "execute command", "reason")
		fs.Int("ttl", 5, "ttl minutes")
		fs.String("intent", "", "intent name")
		fs.String("env-path", "", "optional .env candidate path for approval context")
		fs.String("secrets", "", "comma-separated secret names")
		fs.Bool("auto-approve", false, "approve immediately (demo only; requires PROMPTLOCK_DEV_MODE=1)")
		fs.Duration("wait-approve", 2*time.Minute, "max time to wait for external approval")
		fs.Duration("poll-interval", 2*time.Second, "poll interval while waiting for approval")
		fs.Bool("allow-risky-command", false, "allow risky commands (env/printenv/proc environ reads)")
		fs.Bool("broker-exec", false, "execute command via broker /v1/leases/execute")
		printFlagHelp(os.Stdout, execHelpText(), fs)
		return
	}

	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	agent := fs.String("agent", "agent", "agent id")
	task := fs.String("task", "task", "task id")
	sessionToken := fs.String("session-token", getenv("PROMPTLOCK_SESSION_TOKEN", ""), "agent session token")
	reason := fs.String("reason", "execute command", "reason")
	ttl := fs.Int("ttl", 5, "ttl minutes")
	intent := fs.String("intent", "", "intent name")
	envPath := fs.String("env-path", "", "optional .env candidate path for approval context")
	secretsCSV := fs.String("secrets", "", "comma-separated secret names")
	autoApprove := fs.Bool("auto-approve", false, "approve immediately (demo only; requires PROMPTLOCK_DEV_MODE=1)")
	waitApprove := fs.Duration("wait-approve", 2*time.Minute, "max time to wait for external approval")
	pollInterval := fs.Duration("poll-interval", 2*time.Second, "poll interval while waiting for approval")
	allowRisky := fs.Bool("allow-risky-command", false, "allow risky commands (env/printenv/proc environ reads)")
	brokerExec := fs.Bool("broker-exec", false, "execute command via broker /v1/leases/execute")
	fs.Parse(args)
	broker, err := conn.resolve(brokerRoleAgent)
	if err != nil {
		fatal(err)
	}

	cmdArgs := fs.Args()
	sep := indexOf(cmdArgs, "--")
	if sep >= 0 {
		cmdArgs = cmdArgs[sep+1:]
	}
	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "missing command after --")
		os.Exit(2)
	}
	if !*allowRisky {
		if riskyReason := detectRiskyCommand(cmdArgs); riskyReason != "" {
			fatal(fmt.Errorf("blocked by command policy: %s (use --allow-risky-command to override)", riskyReason))
		}
	}

	secrets := []string{}
	if *intent != "" {
		resolved, err := resolveIntent(broker.BaseURL, broker.UnixSocket, *sessionToken, *intent)
		if err != nil {
			fatal(err)
		}
		secrets = resolved
	} else if *secretsCSV != "" {
		for _, s := range strings.Split(*secretsCSV, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				secrets = append(secrets, s)
			}
		}
	}
	if len(secrets) == 0 {
		fatal(fmt.Errorf("no secrets resolved; use --intent or --secrets"))
	}

	caps, err := brokerCapabilities(broker.BaseURL, broker.UnixSocket)
	if err == nil {
		if err := validateExecCapabilityPreconditions(caps, *sessionToken, *brokerExec); err != nil {
			fatal(err)
		}
	}

	if *brokerExec && strings.TrimSpace(*intent) == "" {
		fatal(fmt.Errorf("--broker-exec requires --intent for intent-aware policy enforcement"))
	}

	fingerprint := commandFingerprint(cmdArgs)
	wdfp, err := workdirFingerprint()
	if err != nil {
		fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	reqID, err := requestLease(broker.BaseURL, broker.UnixSocket, *sessionToken, requestBody{
		AgentID:            *agent,
		TaskID:             *task,
		Intent:             strings.TrimSpace(*intent),
		Reason:             *reason,
		TTLMinutes:         *ttl,
		Secrets:            secrets,
		CommandFingerprint: fingerprint,
		WorkdirFingerprint: wdfp,
		CommandSummary:     app.SummarizeCommandArgs(cmdArgs),
		WorkdirSummary:     app.SummarizeWorkdirPath(cwd),
		EnvPath:            strings.TrimSpace(*envPath),
	})
	if err != nil {
		fatal(err)
	}

	var lease string
	if *autoApprove {
		if getenv("PROMPTLOCK_DEV_MODE", "") != "1" {
			fatal(fmt.Errorf("--auto-approve is disabled unless PROMPTLOCK_DEV_MODE=1"))
		}
		operatorBroker, err := conn.resolve(brokerRoleOperator)
		if err != nil {
			fatal(err)
		}
		lease, err = approve(operatorBroker.BaseURL, operatorBroker.UnixSocket, getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), reqID, *ttl)
		if err != nil {
			fatal(err)
		}
	} else {
		lease, err = waitForApproval(broker.BaseURL, broker.UnixSocket, *sessionToken, reqID, *waitApprove, *pollInterval)
		if err != nil {
			fatal(err)
		}
	}

	if *brokerExec {
		exitCode, output, err := executeWithSecret(broker.BaseURL, broker.UnixSocket, *sessionToken, lease, *intent, cmdArgs, secrets, fingerprint, wdfp)
		if err != nil {
			fatal(err)
		}
		if output != "" {
			fmt.Print(output)
		}
		os.Exit(exitCode)
	}

	leasedSecrets := map[string]string{}
	for _, s := range secrets {
		v, err := accessSecret(broker.BaseURL, broker.UnixSocket, *sessionToken, lease, s, fingerprint, wdfp)
		if err != nil {
			fatal(err)
		}
		leasedSecrets[s] = v
	}

	c := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	c.Env = buildLocalExecutionEnv(leasedSecrets)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fatal(err)
	}
}

func resolveIntent(broker, brokerUnix, sessionToken, intent string) ([]string, error) {
	var out struct {
		Secrets []string `json:"secrets"`
	}
	if err := postJSONAuth(broker, brokerUnix, "/v1/intents/resolve", sessionToken, map[string]string{"intent": intent}, &out); err != nil {
		return nil, err
	}
	return out.Secrets, nil
}

func requestLease(broker, brokerUnix, sessionToken string, req requestBody) (string, error) {
	var out struct {
		RequestID string `json:"request_id"`
	}
	if err := postJSONAuth(broker, brokerUnix, "/v1/leases/request", sessionToken, req, &out); err != nil {
		return "", err
	}
	if out.RequestID == "" {
		return "", fmt.Errorf("empty request_id")
	}
	return out.RequestID, nil
}

func approve(broker, brokerUnix, operatorToken, requestID string, ttl int) (string, error) {
	var out struct {
		LeaseToken string `json:"lease_token"`
	}
	if err := postJSONAuth(broker, brokerUnix, "/v1/leases/approve?request_id="+requestID, operatorToken, map[string]int{"ttl_minutes": ttl}, &out); err != nil {
		return "", err
	}
	if out.LeaseToken == "" {
		return "", fmt.Errorf("empty lease_token")
	}
	return out.LeaseToken, nil
}

func accessSecret(broker, brokerUnix, sessionToken, lease, secret, fingerprint, workdirFP string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	if err := postJSONAuth(broker, brokerUnix, "/v1/leases/access", sessionToken, map[string]string{"lease_token": lease, "secret": secret, "command_fingerprint": fingerprint, "workdir_fingerprint": workdirFP}, &out); err != nil {
		return "", err
	}
	return out.Value, nil
}

func fetchLeaseByRequest(broker, brokerUnix, sessionToken, requestID string) (string, error) {
	resp, err := getAuth(broker, brokerUnix, "/v1/leases/by-request?request_id="+requestID, sessionToken)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", responseError("fetch lease failed", resp)
	}
	var out struct {
		LeaseToken string `json:"lease_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.LeaseToken == "" {
		return "", fmt.Errorf("empty lease token from approved request")
	}
	return out.LeaseToken, nil
}

func requestStatus(broker, brokerUnix, sessionToken, requestID string) (string, error) {
	resp, err := getAuth(broker, brokerUnix, "/v1/requests/status?request_id="+requestID, sessionToken)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", responseError("status check failed", resp)
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Status, nil
}

func waitForApproval(broker, brokerUnix, sessionToken, requestID string, timeout, poll time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		status, err := requestStatus(broker, brokerUnix, sessionToken, requestID)
		if err != nil {
			return "", err
		}
		switch status {
		case "approved":
			return fetchLeaseByRequest(broker, brokerUnix, sessionToken, requestID)
		case "denied":
			return "", fmt.Errorf("request denied: %s", requestID)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("approval timeout for request %s", requestID)
		}
		time.Sleep(poll)
	}
}

func brokerCapabilities(broker, brokerUnix string) (capabilities, error) {
	resp, err := getAuth(broker, brokerUnix, "/v1/meta/capabilities", "")
	if err != nil {
		return capabilities{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return capabilities{}, responseError("capabilities request failed", resp)
	}
	var out capabilities
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return capabilities{}, err
	}
	return out, nil
}

func validateExecCapabilityPreconditions(caps capabilities, sessionToken string, brokerExec bool) error {
	if caps.AuthEnabled && strings.TrimSpace(sessionToken) == "" {
		return fmt.Errorf("broker requires session token; provide --session-token or PROMPTLOCK_SESSION_TOKEN")
	}
	if caps.AuthEnabled && !caps.AllowPlaintextSecretReturn && !brokerExec {
		return fmt.Errorf("broker policy disables plaintext secret return; re-run with --broker-exec")
	}
	return nil
}

func executeWithSecret(broker, brokerUnix, sessionToken, lease, intent string, command, secrets []string, fp, wdfp string) (int, string, error) {
	var out struct {
		ExitCode     int    `json:"exit_code"`
		StdoutStderr string `json:"stdout_stderr"`
	}
	payload := map[string]any{
		"lease_token":         lease,
		"intent":              intent,
		"command":             command,
		"secrets":             secrets,
		"command_fingerprint": fp,
		"workdir_fingerprint": wdfp,
	}
	if err := postJSONAuth(broker, brokerUnix, "/v1/leases/execute", sessionToken, payload, &out); err != nil {
		return 1, "", err
	}
	return out.ExitCode, out.StdoutStderr, nil
}
