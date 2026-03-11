package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/audit"
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

const (
	brokerTLSCAFileEnv         = "PROMPTLOCK_BROKER_TLS_CA_FILE"
	brokerTLSClientCertFileEnv = "PROMPTLOCK_BROKER_TLS_CLIENT_CERT_FILE"
	brokerTLSClientKeyFileEnv  = "PROMPTLOCK_BROKER_TLS_CLIENT_KEY_FILE"
	brokerTLSServerNameEnv     = "PROMPTLOCK_BROKER_TLS_SERVER_NAME"
)

type brokerTLSOptions struct {
	CAFile         string
	ClientCertFile string
	ClientKeyFile  string
	ServerName     string
}

type brokerFlags struct {
	Broker              *string
	BrokerUnix          *string
	BrokerTLSCAFile     *string
	BrokerTLSClientCert *string
	BrokerTLSClientKey  *string
	BrokerTLSServerName *string
}

var activeBrokerTLSOptions = brokerTLSOptions{}

func registerBrokerFlags(fs *flag.FlagSet) brokerFlags {
	return brokerFlags{
		Broker:              fs.String("broker", getenv("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765"), "broker URL"),
		BrokerUnix:          fs.String("broker-unix-socket", getenv("PROMPTLOCK_BROKER_UNIX_SOCKET", ""), "broker unix socket path"),
		BrokerTLSCAFile:     fs.String("broker-tls-ca-file", getenv(brokerTLSCAFileEnv, ""), "custom CA file for HTTPS broker TLS verification"),
		BrokerTLSClientCert: fs.String("broker-tls-client-cert-file", getenv(brokerTLSClientCertFileEnv, ""), "client certificate file for HTTPS mTLS"),
		BrokerTLSClientKey:  fs.String("broker-tls-client-key-file", getenv(brokerTLSClientKeyFileEnv, ""), "client private key file for HTTPS mTLS"),
		BrokerTLSServerName: fs.String("broker-tls-server-name", getenv(brokerTLSServerNameEnv, ""), "expected TLS server name for HTTPS broker"),
	}
}

func (f brokerFlags) applyTLSOptions() {
	activeBrokerTLSOptions = brokerTLSOptions{
		CAFile:         strings.TrimSpace(*f.BrokerTLSCAFile),
		ClientCertFile: strings.TrimSpace(*f.BrokerTLSClientCert),
		ClientKeyFile:  strings.TrimSpace(*f.BrokerTLSClientKey),
		ServerName:     strings.TrimSpace(*f.BrokerTLSServerName),
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: promptlock <exec|approve-queue|audit-verify|auth> [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "exec":
		runExec(os.Args[2:])
	case "approve-queue":
		runApproveQueue(os.Args[2:])
	case "audit-verify":
		runAuditVerify(os.Args[2:])
	case "auth":
		runAuth(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "usage: promptlock <exec|approve-queue|audit-verify|auth> [flags]")
		os.Exit(2)
	}
}

func runExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
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
	conn.applyTLSOptions()

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
		resolved, err := resolveIntent(*conn.Broker, *conn.BrokerUnix, *sessionToken, *intent)
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

	caps, err := brokerCapabilities(*conn.Broker, *conn.BrokerUnix)
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
	reqID, err := requestLease(*conn.Broker, *conn.BrokerUnix, *sessionToken, requestBody{AgentID: *agent, TaskID: *task, Reason: *reason, TTLMinutes: *ttl, Secrets: secrets, CommandFingerprint: fingerprint, WorkdirFingerprint: wdfp})
	if err != nil {
		fatal(err)
	}

	var lease string
	if *autoApprove {
		if getenv("PROMPTLOCK_DEV_MODE", "") != "1" {
			fatal(fmt.Errorf("--auto-approve is disabled unless PROMPTLOCK_DEV_MODE=1"))
		}
		lease, err = approve(*conn.Broker, *conn.BrokerUnix, getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), reqID, *ttl)
		if err != nil {
			fatal(err)
		}
	} else {
		lease, err = waitForApproval(*conn.Broker, *conn.BrokerUnix, *sessionToken, reqID, *waitApprove, *pollInterval)
		if err != nil {
			fatal(err)
		}
	}

	if *brokerExec {
		exitCode, output, err := executeWithSecret(*conn.Broker, *conn.BrokerUnix, *sessionToken, lease, *intent, cmdArgs, secrets, fingerprint, wdfp)
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
		v, err := accessSecret(*conn.Broker, *conn.BrokerUnix, *sessionToken, lease, s, fingerprint, wdfp)
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
	client, err := httpClient(baseURL, unixSocket)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return responseError("request failed", resp)
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
	client, err := httpClient(baseURL, unixSocket)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func buildURL(baseURL, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(baseURL, "/") + path
}

func httpClient(baseURL, unixSocket string) (*http.Client, error) {
	opts := activeBrokerTLSOptions
	if unixSocket != "" {
		if opts.CAFile != "" || opts.ClientCertFile != "" || opts.ClientKeyFile != "" || opts.ServerName != "" {
			return nil, fmt.Errorf("broker tls options are not supported with --broker-unix-socket")
		}
		tr := &http.Transport{}
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", unixSocket)
		}
		return &http.Client{Transport: tr}, nil
	}

	usesTLSOverrides := opts.CAFile != "" || opts.ClientCertFile != "" || opts.ClientKeyFile != "" || opts.ServerName != ""
	if !usesTLSOverrides {
		return http.DefaultClient, nil
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(baseURL)), "https://") {
		return nil, fmt.Errorf("broker tls options require an https broker URL")
	}

	tlsConfig, err := buildBrokerTLSConfig(opts)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}, nil
}

func buildBrokerTLSConfig(opts brokerTLSOptions) (*tls.Config, error) {
	if opts.ClientCertFile != "" && opts.ClientKeyFile == "" {
		return nil, fmt.Errorf("broker tls client cert requires --broker-tls-client-key-file")
	}
	if opts.ClientKeyFile != "" && opts.ClientCertFile == "" {
		return nil, fmt.Errorf("broker tls client key requires --broker-tls-client-cert-file")
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if opts.ServerName != "" {
		tlsConfig.ServerName = opts.ServerName
	}
	if opts.CAFile != "" {
		caPEM, err := os.ReadFile(opts.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read broker tls ca file %s: %w", opts.CAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parse broker tls ca file %s: no certificates found", opts.CAFile)
		}
		tlsConfig.RootCAs = pool
	}
	if opts.ClientCertFile != "" && opts.ClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.ClientCertFile, opts.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load broker tls client certificate/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
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
	conn := registerBrokerFlags(fs)
	poll := fs.Duration("poll-interval", 3*time.Second, "poll interval")
	defaultTTL := fs.Int("ttl", 5, "approval ttl override")
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	once := fs.Bool("once", false, "process one pass and exit")
	fs.Parse(args)
	conn.applyTLSOptions()

	for {
		items, err := listPending(*conn.Broker, *conn.BrokerUnix, *operatorToken)
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
				_, err := approve(*conn.Broker, *conn.BrokerUnix, *operatorToken, it.ID, *defaultTTL)
				if err != nil {
					fmt.Println("approve failed:", err)
				} else {
					fmt.Println("approved")
				}
			case "n", "no":
				if err := deny(*conn.Broker, *conn.BrokerUnix, *operatorToken, it.ID, "denied by operator"); err != nil {
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
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	fs.Parse(args)
	conn.applyTLSOptions()
	items, err := listPending(*conn.Broker, *conn.BrokerUnix, *operatorToken)
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
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	ttl := fs.Int("ttl", 5, "approval ttl override")
	fs.Parse(args)
	conn.applyTLSOptions()
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: promptlock approve-queue allow [--broker URL] [--ttl N] <request_id>"))
	}
	requestID := fs.Arg(0)
	if _, err := approve(*conn.Broker, *conn.BrokerUnix, *operatorToken, requestID, *ttl); err != nil {
		fatal(err)
	}
	fmt.Println("approved", requestID)
}

func runApproveDeny(args []string) {
	fs := flag.NewFlagSet("approve-queue deny", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	reason := fs.String("reason", "denied by operator", "deny reason")
	fs.Parse(args)
	conn.applyTLSOptions()
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: promptlock approve-queue deny [--broker URL] [--reason TEXT] <request_id>"))
	}
	requestID := fs.Arg(0)
	if err := deny(*conn.Broker, *conn.BrokerUnix, *operatorToken, requestID, *reason); err != nil {
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
		return nil, responseError("pending request failed", resp)
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
		return capabilities{}, responseError("capabilities request failed", resp)
	}
	var out capabilities
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return capabilities{}, err
	}
	return out, nil
}

func responseError(prefix string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("%s: %s", prefix, resp.Status)
	}
	return fmt.Errorf("%s: %s (%s)", prefix, msg, resp.Status)
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

func runAuth(args []string) {
	if len(args) == 0 {
		fatal(fmt.Errorf("usage: promptlock auth <bootstrap|pair|mint|login> [flags]"))
	}
	switch args[0] {
	case "bootstrap":
		runAuthBootstrap(args[1:])
	case "pair":
		runAuthPair(args[1:])
	case "mint":
		runAuthMint(args[1:])
	case "login":
		runAuthLogin(args[1:])
	default:
		fatal(fmt.Errorf("usage: promptlock auth <bootstrap|pair|mint|login> [flags]"))
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
	if err := postJSONAuth(broker, brokerUnix, "/v1/auth/bootstrap/create", operatorToken, map[string]string{"agent_id": agentID, "container_id": containerID}, &out); err != nil {
		return authBootstrapResult{}, err
	}
	if strings.TrimSpace(out.BootstrapToken) == "" {
		return authBootstrapResult{}, fmt.Errorf("empty bootstrap_token")
	}
	return out, nil
}

func authPair(broker, brokerUnix, token, containerID string) (authPairResult, error) {
	var out authPairResult
	if err := postJSONAuth(broker, brokerUnix, "/v1/auth/pair/complete", "", map[string]string{"token": token, "container_id": containerID}, &out); err != nil {
		return authPairResult{}, err
	}
	if strings.TrimSpace(out.GrantID) == "" {
		return authPairResult{}, fmt.Errorf("empty grant_id")
	}
	return out, nil
}

func authMint(broker, brokerUnix, grantID string) (authMintResult, error) {
	var out authMintResult
	if err := postJSONAuth(broker, brokerUnix, "/v1/auth/session/mint", "", map[string]string{"grant_id": grantID}, &out); err != nil {
		return authMintResult{}, err
	}
	if strings.TrimSpace(out.SessionToken) == "" {
		return authMintResult{}, fmt.Errorf("empty session_token")
	}
	return out, nil
}

func authLogin(broker, brokerUnix, operatorToken, agentID, containerID string) (authLoginResult, error) {
	bootstrap, err := authBootstrap(broker, brokerUnix, operatorToken, agentID, containerID)
	if err != nil {
		return authLoginResult{}, fmt.Errorf("bootstrap step failed: %w", err)
	}
	pair, err := authPair(broker, brokerUnix, bootstrap.BootstrapToken, containerID)
	if err != nil {
		return authLoginResult{}, fmt.Errorf("pair step failed: %w", err)
	}
	mint, err := authMint(broker, brokerUnix, pair.GrantID)
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
	conn.applyTLSOptions()
	if strings.TrimSpace(*opToken) == "" || strings.TrimSpace(*agent) == "" || strings.TrimSpace(*container) == "" {
		fatal(fmt.Errorf("--operator-token, --agent and --container are required"))
	}
	out, err := authBootstrap(*conn.Broker, *conn.BrokerUnix, *opToken, *agent, *container)
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
	conn.applyTLSOptions()
	if strings.TrimSpace(*token) == "" || strings.TrimSpace(*container) == "" {
		fatal(fmt.Errorf("--token and --container are required"))
	}
	out, err := authPair(*conn.Broker, *conn.BrokerUnix, *token, *container)
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
	conn.applyTLSOptions()
	if strings.TrimSpace(*grant) == "" {
		fatal(fmt.Errorf("--grant is required"))
	}
	out, err := authMint(*conn.Broker, *conn.BrokerUnix, *grant)
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
	fs.Parse(args)
	conn.applyTLSOptions()
	if strings.TrimSpace(*opToken) == "" || strings.TrimSpace(*agent) == "" || strings.TrimSpace(*container) == "" {
		fatal(fmt.Errorf("--operator-token, --agent and --container are required"))
	}
	out, err := authLogin(*conn.Broker, *conn.BrokerUnix, *opToken, *agent, *container)
	if err != nil {
		fatal(err)
	}
	writeJSONStdout(map[string]any{
		"session_token": out.SessionToken,
		"expires_at":    out.ExpiresAt,
		"grant_id":      out.GrantID,
	})
}

func writeJSONStdout(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func runAuditVerify(args []string) {
	fs := flag.NewFlagSet("audit-verify", flag.ExitOnError)
	auditPath := fs.String("file", "", "path to audit jsonl file")
	checkpoint := fs.String("checkpoint", "", "optional checkpoint file path")
	writeCheckpoint := fs.Bool("write-checkpoint", false, "write/refresh checkpoint with latest verified hash")
	fs.Parse(args)
	if strings.TrimSpace(*auditPath) == "" {
		fatal(fmt.Errorf("--file is required"))
	}
	last, count, err := audit.VerifyFile(*auditPath)
	if err != nil {
		fatal(err)
	}
	if *checkpoint != "" {
		if prev, err := audit.ReadCheckpoint(*checkpoint); err == nil && strings.TrimSpace(prev) != "" && prev != last {
			fatal(fmt.Errorf("checkpoint mismatch: expected %s got %s", prev, last))
		}
		if *writeCheckpoint {
			if err := audit.WriteCheckpoint(*checkpoint, last); err != nil {
				fatal(err)
			}
		}
	}
	fmt.Printf("audit verify ok: records=%d last_hash=%s\n", count, last)
}
