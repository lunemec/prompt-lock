package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/audit"
	"github.com/lunemec/promptlock/internal/config"
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
	EnvPath            string   `json:"env_path,omitempty"`
}

type capabilities struct {
	AuthEnabled                bool `json:"auth_enabled"`
	AllowPlaintextSecretReturn bool `json:"allow_plaintext_secret_return"`
}

type brokerFlags struct {
	Broker     *string
	BrokerUnix *string
}

var doPostJSONAuth = postJSONAuth
var brokerClientTimeout = 10 * time.Second

const (
	defaultBrokerURL         = "http://127.0.0.1:8765"
	unixSocketRequestBaseURL = "http://promptlock"
)

type brokerRole string

const (
	brokerRoleAgent    brokerRole = "agent"
	brokerRoleOperator brokerRole = "operator"
)

type brokerSelectionInput struct {
	BaseURL    string
	UnixSocket string
}

type brokerSelection struct {
	BaseURL    string
	UnixSocket string
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func registerBrokerFlags(fs *flag.FlagSet) brokerFlags {
	return brokerFlags{
		Broker:     fs.String("broker", "", "broker URL"),
		BrokerUnix: fs.String("broker-unix-socket", "", "broker unix socket path"),
	}
}

func (f brokerFlags) resolve(role brokerRole) (brokerSelection, error) {
	return resolveBrokerSelection(role, brokerSelectionInput{
		BaseURL:    strings.TrimSpace(*f.Broker),
		UnixSocket: strings.TrimSpace(*f.BrokerUnix),
	})
}

func resolveBrokerSelection(role brokerRole, in brokerSelectionInput) (brokerSelection, error) {
	explicitURL := strings.TrimSpace(in.BaseURL)
	explicitUnix := strings.TrimSpace(in.UnixSocket)
	if explicitUnix != "" {
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: explicitUnix,
		}, nil
	}
	if explicitURL != "" {
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: "",
		}, nil
	}
	if compatUnix := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_UNIX_SOCKET")); compatUnix != "" {
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: compatUnix,
		}, nil
	}
	if roleUnix := roleSpecificBrokerUnixSocket(role); roleUnix != "" {
		if brokerSocketExists(roleUnix) {
			return brokerSelection{
				BaseURL:    normalizeBrokerURL(explicitURL),
				UnixSocket: roleUnix,
			}, nil
		}
		if envURL := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_URL")); envURL != "" {
			return brokerSelection{
				BaseURL:    envURL,
				UnixSocket: "",
			}, nil
		}
		return brokerSelection{}, fmt.Errorf("%s broker unix socket not found at %s; set --broker or PROMPTLOCK_BROKER_URL for explicit TCP transport", role, roleUnix)
	}
	defaultUnix := defaultBrokerUnixSocket(role)
	if brokerSocketExists(defaultUnix) {
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: defaultUnix,
		}, nil
	}
	if envURL := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_URL")); envURL != "" {
		return brokerSelection{
			BaseURL:    envURL,
			UnixSocket: "",
		}, nil
	}
	return brokerSelection{}, fmt.Errorf("%s broker unix socket not found at %s; set --broker or PROMPTLOCK_BROKER_URL for explicit TCP transport", role, defaultUnix)
}

func normalizeBrokerURL(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if envURL := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_URL")); envURL != "" {
		return envURL
	}
	return defaultBrokerURL
}

func roleSpecificBrokerUnixSocket(role brokerRole) string {
	switch role {
	case brokerRoleOperator:
		return strings.TrimSpace(os.Getenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET"))
	case brokerRoleAgent:
		return strings.TrimSpace(os.Getenv("PROMPTLOCK_AGENT_UNIX_SOCKET"))
	default:
		return ""
	}
}

func defaultBrokerUnixSocket(role brokerRole) string {
	switch role {
	case brokerRoleOperator:
		return config.DefaultOperatorUnixSocketPath
	case brokerRoleAgent:
		return config.DefaultAgentUnixSocketPath
	default:
		return ""
	}
}

func brokerSocketExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: promptlock <exec|watch|audit-verify|auth> [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "exec":
		runExec(os.Args[2:])
	case "watch":
		runWatch(os.Args[2:])
	case "audit-verify":
		runAuditVerify(os.Args[2:])
	case "auth":
		runAuth(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "usage: promptlock <exec|watch|audit-verify|auth> [flags]")
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
	reqID, err := requestLease(broker.BaseURL, broker.UnixSocket, *sessionToken, requestBody{
		AgentID:            *agent,
		TaskID:             *task,
		Intent:             strings.TrimSpace(*intent),
		Reason:             *reason,
		TTLMinutes:         *ttl,
		Secrets:            secrets,
		CommandFingerprint: fingerprint,
		WorkdirFingerprint: wdfp,
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

func postJSONAuth(baseURL, unixSocket, path, bearer string, in any, out any) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequest(http.MethodPost, buildURL(baseURL, unixSocket, path), bytes.NewReader(b))
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
		return normalizeBrokerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return responseError("request failed", resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getAuth(baseURL, unixSocket, path, bearer string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, buildURL(baseURL, unixSocket, path), nil)
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, normalizeBrokerRequestError(err)
	}
	return resp, nil
}

func buildURL(baseURL, unixSocket, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.TrimSpace(unixSocket) != "" {
		baseURL = unixSocketRequestBaseURL
	}
	return strings.TrimRight(baseURL, "/") + path
}

func httpClient(baseURL, unixSocket string) (*http.Client, error) {
	if unixSocket != "" {
		tr := &http.Transport{}
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", unixSocket)
		}
		return &http.Client{Transport: tr, Timeout: brokerClientTimeout}, nil
	}
	return &http.Client{Timeout: brokerClientTimeout}, nil
}

func normalizeBrokerRequestError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("broker request timed out after %s", brokerClientTimeout)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("broker request timed out after %s", brokerClientTimeout)
	}
	return err
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

type pendingItem struct {
	ID                 string   `json:"ID"`
	AgentID            string   `json:"AgentID"`
	TaskID             string   `json:"TaskID"`
	Intent             string   `json:"Intent"`
	Reason             string   `json:"Reason"`
	TTLMinutes         int      `json:"TTLMinutes"`
	Secrets            []string `json:"Secrets"`
	CommandFingerprint string   `json:"CommandFingerprint"`
	WorkdirFingerprint string   `json:"WorkdirFingerprint"`
	EnvPath            string   `json:"EnvPath"`
	EnvPathCanonical   string   `json:"EnvPathCanonical"`
}

type pendingResponse struct {
	Pending []pendingItem `json:"pending"`
}

const ansiClearScreen = "\x1b[2J\x1b[H"

type watchClient interface {
	ListPending() ([]pendingItem, error)
	Approve(requestID string, ttl int) error
	Deny(requestID, reason string) error
}

type brokerWatchClient struct {
	broker        string
	brokerUnix    string
	operatorToken string
}

func (c brokerWatchClient) ListPending() ([]pendingItem, error) {
	return listPending(c.broker, c.brokerUnix, c.operatorToken)
}

func (c brokerWatchClient) Approve(requestID string, ttl int) error {
	_, err := approve(c.broker, c.brokerUnix, c.operatorToken, requestID, ttl)
	return err
}

func (c brokerWatchClient) Deny(requestID, reason string) error {
	return deny(c.broker, c.brokerUnix, c.operatorToken, requestID, reason)
}

type watchOptions struct {
	BrokerTarget string
	PollInterval time.Duration
	DefaultTTL   int
	Once         bool
	Input        *bufio.Reader
	Output       io.Writer
	ClearScreen  bool
	Now          func() time.Time
	Pause        func(time.Duration)
}

type watchView struct {
	BrokerTarget string
	PollInterval time.Duration
	Now          time.Time
	PendingCount int
	Current      *pendingItem
	MoreQueued   int
	Message      string
}

func runWatch(args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			runWatchList(args[1:])
			return
		case "allow":
			runWatchAllow(args[1:])
			return
		case "deny":
			runWatchDeny(args[1:])
			return
		}
	}

	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	poll := fs.Duration("poll-interval", 3*time.Second, "poll interval")
	defaultTTL := fs.Int("ttl", 5, "approval ttl override")
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	once := fs.Bool("once", false, "process one pass and exit")
	fs.Parse(args)
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	client := brokerWatchClient{
		broker:        broker.BaseURL,
		brokerUnix:    broker.UnixSocket,
		operatorToken: *operatorToken,
	}
	opts := watchOptions{
		BrokerTarget: watchBrokerTarget(broker.BaseURL, broker.UnixSocket),
		PollInterval: *poll,
		DefaultTTL:   *defaultTTL,
		Once:         *once,
		Input:        bufio.NewReader(os.Stdin),
		Output:       os.Stdout,
		ClearScreen:  isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout),
		Now:          time.Now,
		Pause:        time.Sleep,
	}
	if err := runWatchLoop(client, opts); err != nil {
		fatal(err)
	}
}

func runWatchLoop(client watchClient, opts watchOptions) error {
	if opts.Input == nil {
		opts.Input = bufio.NewReader(os.Stdin)
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Pause == nil {
		opts.Pause = time.Sleep
	}
	state := watchLoopState{skipped: map[string]struct{}{}}

	for {
		items, err := client.ListPending()
		if err != nil {
			return err
		}

		membershipSignature := pendingMembershipSignature(items)
		if membershipSignature != state.membershipSignature {
			state.membershipSignature = membershipSignature
			state.skipped = map[string]struct{}{}
		}

		current, moreQueued, message := selectWatchItem(items, state.skipped)
		renderKey := watchRenderKey(membershipSignature, current, moreQueued, message)
		if renderKey != state.lastRenderKey {
			renderWatchScreen(opts.Output, watchView{
				BrokerTarget: opts.BrokerTarget,
				PollInterval: opts.PollInterval,
				Now:          opts.Now(),
				PendingCount: len(items),
				Current:      current,
				MoreQueued:   moreQueued,
				Message:      message,
			}, opts.ClearScreen)
			state.lastRenderKey = renderKey
		}

		if current == nil {
			if opts.Once {
				return nil
			}
			opts.Pause(opts.PollInterval)
			continue
		}

		decision, err := promptWatchDecision(opts.Input, opts.Output)
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(opts.Output, "\nstdin closed; leaving pending requests untouched")
				return nil
			}
			return err
		}

		switch decision {
		case "approve":
			if err := client.Approve(current.ID, opts.DefaultTTL); err != nil {
				fmt.Fprintln(opts.Output, "approve failed:", err)
			} else {
				fmt.Fprintln(opts.Output, "approved")
			}
			state.lastRenderKey = ""
		case "deny":
			if err := client.Deny(current.ID, "denied by operator"); err != nil {
				fmt.Fprintln(opts.Output, "deny failed:", err)
			} else {
				fmt.Fprintln(opts.Output, "denied")
			}
			state.lastRenderKey = ""
		case "skip":
			state.skipped[current.ID] = struct{}{}
			state.lastRenderKey = ""
		case "quit":
			fmt.Fprintln(opts.Output, "watch exited; leaving pending requests untouched")
			return nil
		}
	}
}

type watchLoopState struct {
	membershipSignature string
	skipped             map[string]struct{}
	lastRenderKey       string
}

func promptWatchDecision(input *bufio.Reader, out io.Writer) (string, error) {
	for {
		fmt.Fprint(out, "Action? [y]es / [n]o / [s]kip / [q]uit: ")
		line, err := input.ReadString('\n')
		trimmed := strings.ToLower(strings.TrimSpace(line))
		switch trimmed {
		case "y", "yes":
			return "approve", nil
		case "n", "no":
			return "deny", nil
		case "s", "skip":
			return "skip", nil
		case "q", "quit":
			return "quit", nil
		case "":
			if errors.Is(err, io.EOF) {
				return "", io.EOF
			}
			fmt.Fprintln(out, "Enter y, n, s, or q.")
			continue
		default:
			if errors.Is(err, io.EOF) {
				return "", io.EOF
			}
			fmt.Fprintln(out, "Enter y, n, s, or q.")
			continue
		}
	}
}

func runWatchList(args []string) {
	fs := flag.NewFlagSet("watch list", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	fs.Parse(args)
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	items, err := listPending(broker.BaseURL, broker.UnixSocket, *operatorToken)
	if err != nil {
		fatal(err)
	}
	if len(items) == 0 {
		fmt.Println("no pending requests")
		return
	}
	for _, it := range items {
		fmt.Println(formatPendingListLine(it))
	}
}

func runWatchAllow(args []string) {
	fs := flag.NewFlagSet("watch allow", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	ttl := fs.Int("ttl", 5, "approval ttl override")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: promptlock watch allow [--broker URL] [--ttl N] <request_id>"))
	}
	requestID := fs.Arg(0)
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	if _, err := approve(broker.BaseURL, broker.UnixSocket, *operatorToken, requestID, *ttl); err != nil {
		fatal(err)
	}
	fmt.Println("approved", requestID)
}

func runWatchDeny(args []string) {
	fs := flag.NewFlagSet("watch deny", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	reason := fs.String("reason", "denied by operator", "deny reason")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: promptlock watch deny [--broker URL] [--reason TEXT] <request_id>"))
	}
	requestID := fs.Arg(0)
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	if err := deny(broker.BaseURL, broker.UnixSocket, *operatorToken, requestID, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("denied", requestID)
}

func listPending(broker, brokerUnix, operatorToken string) ([]pendingItem, error) {
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

func selectWatchItem(items []pendingItem, skipped map[string]struct{}) (*pendingItem, int, string) {
	for idx := range items {
		if _, ok := skipped[items[idx].ID]; ok {
			continue
		}
		moreQueued := unskippedPendingCount(items, skipped) - 1
		return &items[idx], moreQueued, ""
	}
	if len(items) == 0 {
		return nil, 0, "Watching for pending requests..."
	}
	return nil, 0, "All current pending requests have been skipped. Watching for queue changes..."
}

func unskippedPendingCount(items []pendingItem, skipped map[string]struct{}) int {
	count := 0
	for _, item := range items {
		if _, ok := skipped[item.ID]; ok {
			continue
		}
		count++
	}
	return count
}

func renderWatchScreen(out io.Writer, view watchView, clearScreen bool) {
	if clearScreen {
		fmt.Fprint(out, ansiClearScreen)
	}
	fmt.Fprintln(out, "PromptLock Watch")
	fmt.Fprintf(out, "Broker: %s | Poll: %s | Time: %s | Pending: %d\n", view.BrokerTarget, view.PollInterval, view.Now.Format(time.RFC3339), view.PendingCount)
	fmt.Fprintln(out)
	if view.Current == nil {
		fmt.Fprintln(out, view.Message)
		if view.PendingCount == 0 {
			fmt.Fprintln(out, "Waiting for the next approval request.")
		}
		return
	}

	it := view.Current
	fmt.Fprintf(out, "Request %s | agent=%s task=%s ttl=%d\n", it.ID, it.AgentID, it.TaskID, it.TTLMinutes)
	if strings.TrimSpace(it.Intent) != "" {
		fmt.Fprintf(out, "Intent: %s\n", it.Intent)
	}
	fmt.Fprintf(out, "Reason: %s\n", it.Reason)
	fmt.Fprintf(out, "Secrets: %s\n", strings.Join(it.Secrets, ", "))
	fmt.Fprintf(out, "Command FP: %s\n", it.CommandFingerprint)
	fmt.Fprintf(out, "Workdir FP: %s\n", it.WorkdirFingerprint)
	if strings.TrimSpace(it.EnvPath) != "" {
		fmt.Fprintf(out, "Env Path: %s\n", it.EnvPath)
	}
	if strings.TrimSpace(it.EnvPathCanonical) != "" {
		fmt.Fprintf(out, "Env Path Canonical: %s\n", it.EnvPathCanonical)
	}
	if view.MoreQueued > 0 {
		fmt.Fprintf(out, "Queued after this: %d\n", view.MoreQueued)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Action: y=yes n=no s=skip q=quit")
}

func watchRenderKey(membershipSignature string, current *pendingItem, moreQueued int, message string) string {
	if current == nil {
		return "waiting|" + membershipSignature + "|" + message
	}
	return fmt.Sprintf("item|%s|%s|%d", membershipSignature, current.ID, moreQueued)
}

func pendingMembershipSignature(items []pendingItem) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, "\x00")
}

func watchBrokerTarget(broker, brokerUnix string) string {
	if strings.TrimSpace(brokerUnix) != "" {
		return "unix://" + brokerUnix
	}
	return broker
}

func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func formatPendingListLine(it pendingItem) string {
	line := fmt.Sprintf("%s | agent=%s task=%s ttl=%d",
		it.ID,
		it.AgentID,
		it.TaskID,
		it.TTLMinutes,
	)
	if strings.TrimSpace(it.Intent) != "" {
		line += " | intent=" + it.Intent
	}
	line += fmt.Sprintf(" | secrets=%s | reason=%s | command_fp=%s | workdir_fp=%s",
		strings.Join(it.Secrets, ","),
		it.Reason,
		it.CommandFingerprint,
		it.WorkdirFingerprint,
	)
	if strings.TrimSpace(it.EnvPath) != "" {
		line += " | env_path=" + it.EnvPath
	}
	if strings.TrimSpace(it.EnvPathCanonical) != "" {
		line += " | env_path_canonical=" + it.EnvPathCanonical
	}
	return line
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
		fatal(fmt.Errorf("usage: promptlock auth <bootstrap|pair|mint|login|docker-run> [flags]"))
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
	case "docker-run":
		runAuthDockerRun(args[1:])
	default:
		fatal(fmt.Errorf("usage: promptlock auth <bootstrap|pair|mint|login|docker-run> [flags]"))
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
	fs := flag.NewFlagSet("auth docker-run", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	opToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	agent := fs.String("agent", "", "agent id")
	container := fs.String("container", "", "container id / docker name")
	image := fs.String("image", "", "docker image to run")
	containerSocket := fs.String("container-broker-socket", "/run/promptlock/promptlock-agent.sock", "agent broker unix socket path inside the container when using --broker-unix-socket")
	entrypoint := fs.String("entrypoint", "", "optional docker entrypoint override")
	workdir := fs.String("workdir", "", "optional working directory inside container")
	var mounts stringSliceFlag
	var envs stringSliceFlag
	var dockerArgs stringSliceFlag
	fs.Var(&mounts, "mount", "additional docker --mount spec (repeatable)")
	fs.Var(&envs, "env", "additional container env KEY=VALUE (repeatable)")
	fs.Var(&dockerArgs, "docker-arg", "additional raw docker run argument (repeatable)")
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

func currentUserDockerIdentity() string {
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
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
	if *checkpoint != "" {
		if prev, err := audit.ReadCheckpoint(*checkpoint); err == nil && strings.TrimSpace(prev) != "" {
			last, count, err := audit.VerifyFileAnchored(*auditPath, prev)
			if err != nil {
				fatal(err)
			}
			if *writeCheckpoint {
				if err := audit.WriteCheckpoint(*checkpoint, last); err != nil {
					fatal(err)
				}
			}
			fmt.Printf("audit verify ok: records=%d last_hash=%s\n", count, last)
			return
		}
	}
	last, count, err := audit.VerifyFile(*auditPath)
	if err != nil {
		fatal(err)
	}
	if *checkpoint != "" && *writeCheckpoint {
		if err := audit.WriteCheckpoint(*checkpoint, last); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("audit verify ok: records=%d last_hash=%s\n", count, last)
}
