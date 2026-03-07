package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type requestBody struct {
	AgentID            string   `json:"agent_id"`
	TaskID             string   `json:"task_id"`
	Reason             string   `json:"reason"`
	TTLMinutes         int      `json:"ttl_minutes"`
	Secrets            []string `json:"secrets"`
	CommandFingerprint string   `json:"command_fingerprint"`
	WorkdirFingerprint string   `json:"workdir_fingerprint"`
}

type capabilities struct {
	AuthEnabled                bool `json:"auth_enabled"`
	AllowPlaintextSecretReturn bool `json:"allow_plaintext_secret_return"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: promptlock <exec|approve-queue> [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "exec":
		runExec(os.Args[2:])
	case "approve-queue":
		runApproveQueue(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "usage: promptlock <exec|approve-queue> [flags]")
		os.Exit(2)
	}
}

func runExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	broker := fs.String("broker", getenv("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765"), "broker URL")
	brokerUnix := fs.String("broker-unix-socket", getenv("PROMPTLOCK_BROKER_UNIX_SOCKET", ""), "broker unix socket path")
	agent := fs.String("agent", "agent", "agent id")
	task := fs.String("task", "task", "task id")
	sessionToken := fs.String("session-token", getenv("PROMPTLOCK_SESSION_TOKEN", ""), "agent session token")
	reason := fs.String("reason", "execute command", "reason")
	ttl := fs.Int("ttl", 5, "ttl minutes")
	intent := fs.String("intent", "", "intent name")
	secretsCSV := fs.String("secrets", "", "comma-separated secret names")
	autoApprove := fs.Bool("auto-approve", false, "approve immediately (demo only; requires PROMPTLOCK_DEV_MODE=1)")
	waitApprove := fs.Duration("wait-approve", 2*time.Minute, "max time to wait for external approval")
	pollInterval := fs.Duration("poll-interval", 2*time.Second, "poll interval while waiting for approval")
	allowRisky := fs.Bool("allow-risky-command", false, "allow risky commands (env/printenv/proc environ reads)")
	brokerExec := fs.Bool("broker-exec", false, "execute command via broker /v1/leases/execute")
	fs.Parse(args)

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
		resolved, err := resolveIntent(*broker, *brokerUnix, *sessionToken, *intent)
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

	caps, err := brokerCapabilities(*broker, *brokerUnix)
	if err == nil {
		if caps.AuthEnabled && *sessionToken == "" {
			fatal(fmt.Errorf("broker requires session token; provide --session-token or PROMPTLOCK_SESSION_TOKEN"))
		}
		if caps.AuthEnabled && !caps.AllowPlaintextSecretReturn && !*brokerExec {
			fatal(fmt.Errorf("broker policy disables plaintext secret return; re-run with --broker-exec"))
		}
	}

	fingerprint := commandFingerprint(cmdArgs)
	wdfp, err := workdirFingerprint()
	if err != nil {
		fatal(err)
	}
	reqID, err := requestLease(*broker, *brokerUnix, *sessionToken, requestBody{AgentID: *agent, TaskID: *task, Reason: *reason, TTLMinutes: *ttl, Secrets: secrets, CommandFingerprint: fingerprint, WorkdirFingerprint: wdfp})
	if err != nil {
		fatal(err)
	}

	var lease string
	if *autoApprove {
		if getenv("PROMPTLOCK_DEV_MODE", "") != "1" {
			fatal(fmt.Errorf("--auto-approve is disabled unless PROMPTLOCK_DEV_MODE=1"))
		}
		lease, err = approve(*broker, *brokerUnix, getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), reqID, *ttl)
		if err != nil {
			fatal(err)
		}
	} else {
		lease, err = waitForApproval(*broker, *brokerUnix, *sessionToken, reqID, *waitApprove, *pollInterval)
		if err != nil {
			fatal(err)
		}
	}

	if *brokerExec {
		exitCode, output, err := executeWithSecret(*broker, *brokerUnix, *sessionToken, lease, cmdArgs, secrets, fingerprint, wdfp)
		if err != nil {
			fatal(err)
		}
		if output != "" {
			fmt.Print(output)
		}
		os.Exit(exitCode)
	}

	env := os.Environ()
	for _, s := range secrets {
		v, err := accessSecret(*broker, *brokerUnix, *sessionToken, lease, s, fingerprint, wdfp)
		if err != nil {
			fatal(err)
		}
		envName := strings.ToUpper(s)
		env = append(env, envName+"="+v)
	}

	c := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	c.Env = env
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

func postJSONAuth(baseURL, unixSocket, path, bearer string, in any, out any) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequest(http.MethodPost, buildURL(baseURL, path), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := httpClient(unixSocket).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getAuth(baseURL, unixSocket, path, bearer string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, buildURL(baseURL, path), nil)
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return httpClient(unixSocket).Do(req)
}

func buildURL(baseURL, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(baseURL, "/") + path
}

func httpClient(unixSocket string) *http.Client {
	if unixSocket == "" {
		return http.DefaultClient
	}
	tr := &http.Transport{}
	tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", unixSocket)
	}
	return &http.Client{Transport: tr}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

func commandFingerprint(cmd []string) string {
	s := strings.Join(cmd, "\x00")
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func workdirFingerprint() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(wd))
	return hex.EncodeToString(h[:]), nil
}

func detectRiskyCommand(cmd []string) string {
	joined := strings.ToLower(strings.Join(cmd, " "))
	risky := []string{"printenv", " env", "/proc/", "environ", "set "}
	for _, r := range risky {
		if strings.Contains(joined, r) {
			return fmt.Sprintf("contains risky pattern %q", strings.TrimSpace(r))
		}
	}
	return ""
}

func fetchLeaseByRequest(broker, brokerUnix, sessionToken, requestID string) (string, error) {
	resp, err := getAuth(broker, brokerUnix, "/v1/leases/by-request?request_id="+requestID, sessionToken)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch lease failed: %s", resp.Status)
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
		return "", fmt.Errorf("status check failed: %s", resp.Status)
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

type pendingResponse struct {
	Pending []struct {
		ID         string   `json:"ID"`
		AgentID    string   `json:"AgentID"`
		TaskID     string   `json:"TaskID"`
		Reason     string   `json:"Reason"`
		TTLMinutes int      `json:"TTLMinutes"`
		Secrets    []string `json:"Secrets"`
	} `json:"pending"`
}

func runApproveQueue(args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			runApproveList(args[1:])
			return
		case "allow":
			runApproveAllow(args[1:])
			return
		case "deny":
			runApproveDeny(args[1:])
			return
		}
	}

	fs := flag.NewFlagSet("approve-queue", flag.ExitOnError)
	broker := fs.String("broker", getenv("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765"), "broker URL")
	brokerUnix := fs.String("broker-unix-socket", getenv("PROMPTLOCK_BROKER_UNIX_SOCKET", ""), "broker unix socket path")
	poll := fs.Duration("poll-interval", 3*time.Second, "poll interval")
	defaultTTL := fs.Int("ttl", 5, "approval ttl override")
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	once := fs.Bool("once", false, "process one pass and exit")
	fs.Parse(args)

	for {
		items, err := listPending(*broker, *brokerUnix, *operatorToken)
		if err != nil {
			fatal(err)
		}
		for _, it := range items {
			fmt.Printf("\nRequest %s | agent=%s task=%s ttl=%d\n", it.ID, it.AgentID, it.TaskID, it.TTLMinutes)
			fmt.Printf("Reason: %s\n", it.Reason)
			fmt.Printf("Secrets: %s\n", strings.Join(it.Secrets, ", "))
			fmt.Print("Approve? [y]es / [n]o / [s]kip: ")
			var ans string
			_, _ = fmt.Fscanln(os.Stdin, &ans)
			switch strings.ToLower(strings.TrimSpace(ans)) {
			case "y", "yes":
				_, err := approve(*broker, *brokerUnix, *operatorToken, it.ID, *defaultTTL)
				if err != nil {
					fmt.Println("approve failed:", err)
				} else {
					fmt.Println("approved")
				}
			case "n", "no":
				if err := deny(*broker, *brokerUnix, *operatorToken, it.ID, "denied by operator"); err != nil {
					fmt.Println("deny failed:", err)
				} else {
					fmt.Println("denied")
				}
			default:
				fmt.Println("skipped")
			}
		}
		if *once {
			return
		}
		time.Sleep(*poll)
	}
}

func runApproveList(args []string) {
	fs := flag.NewFlagSet("approve-queue list", flag.ExitOnError)
	broker := fs.String("broker", getenv("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765"), "broker URL")
	brokerUnix := fs.String("broker-unix-socket", getenv("PROMPTLOCK_BROKER_UNIX_SOCKET", ""), "broker unix socket path")
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	fs.Parse(args)
	items, err := listPending(*broker, *brokerUnix, *operatorToken)
	if err != nil {
		fatal(err)
	}
	if len(items) == 0 {
		fmt.Println("no pending requests")
		return
	}
	for _, it := range items {
		fmt.Printf("%s | agent=%s task=%s ttl=%d | secrets=%s | reason=%s\n", it.ID, it.AgentID, it.TaskID, it.TTLMinutes, strings.Join(it.Secrets, ","), it.Reason)
	}
}

func runApproveAllow(args []string) {
	fs := flag.NewFlagSet("approve-queue allow", flag.ExitOnError)
	broker := fs.String("broker", getenv("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765"), "broker URL")
	brokerUnix := fs.String("broker-unix-socket", getenv("PROMPTLOCK_BROKER_UNIX_SOCKET", ""), "broker unix socket path")
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	ttl := fs.Int("ttl", 5, "approval ttl override")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: promptlock approve-queue allow [--broker URL] [--ttl N] <request_id>"))
	}
	requestID := fs.Arg(0)
	if _, err := approve(*broker, *brokerUnix, *operatorToken, requestID, *ttl); err != nil {
		fatal(err)
	}
	fmt.Println("approved", requestID)
}

func runApproveDeny(args []string) {
	fs := flag.NewFlagSet("approve-queue deny", flag.ExitOnError)
	broker := fs.String("broker", getenv("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765"), "broker URL")
	brokerUnix := fs.String("broker-unix-socket", getenv("PROMPTLOCK_BROKER_UNIX_SOCKET", ""), "broker unix socket path")
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	reason := fs.String("reason", "denied by operator", "deny reason")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: promptlock approve-queue deny [--broker URL] [--reason TEXT] <request_id>"))
	}
	requestID := fs.Arg(0)
	if err := deny(*broker, *brokerUnix, *operatorToken, requestID, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("denied", requestID)
}

func listPending(broker, brokerUnix, operatorToken string) ([]struct {
	ID         string   `json:"ID"`
	AgentID    string   `json:"AgentID"`
	TaskID     string   `json:"TaskID"`
	Reason     string   `json:"Reason"`
	TTLMinutes int      `json:"TTLMinutes"`
	Secrets    []string `json:"Secrets"`
}, error) {
	resp, err := getAuth(broker, brokerUnix, "/v1/requests/pending", operatorToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pending request failed: %s", resp.Status)
	}
	var out pendingResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Pending, nil
}

func deny(broker, brokerUnix, operatorToken, requestID, reason string) error {
	var out map[string]any
	return postJSONAuth(broker, brokerUnix, "/v1/leases/deny?request_id="+requestID, operatorToken, map[string]string{"reason": reason}, &out)
}

func brokerCapabilities(broker, brokerUnix string) (capabilities, error) {
	resp, err := getAuth(broker, brokerUnix, "/v1/meta/capabilities", "")
	if err != nil {
		return capabilities{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return capabilities{}, fmt.Errorf("capabilities request failed: %s", resp.Status)
	}
	var out capabilities
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return capabilities{}, err
	}
	return out, nil
}

func executeWithSecret(broker, brokerUnix, sessionToken, lease string, command, secrets []string, fp, wdfp string) (int, string, error) {
	var out struct {
		ExitCode     int    `json:"exit_code"`
		StdoutStderr string `json:"stdout_stderr"`
	}
	payload := map[string]any{
		"lease_token":         lease,
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
