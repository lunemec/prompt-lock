package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type result struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Status   int    `json:"status,omitempty"`
	Expected int    `json:"expected,omitempty"`
	Detail   string `json:"detail,omitempty"`
	LogTail  string `json:"log_tail,omitempty"`
}

type report struct {
	OK      bool     `json:"ok"`
	Results []result `json:"results"`
}

type harnessTransport struct {
	operatorClient *http.Client
	operatorBase   string
	agentClient    *http.Client
	agentBase      string
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	outPath := ""
	profile := "dev"
	if len(args) > 0 {
		outPath = args[0]
	}
	if len(args) > 1 {
		profile = args[1]
	}

	rep, code := run(outPath, profile)
	return emitReport(rep, code, outPath, stdout, stderr)
}

func emitReport(rep report, code int, outPath string, stdout, stderr io.Writer) int {
	rendered, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Fprintln(stdout, string(rendered))
	if outPath != "" {
		if err := os.WriteFile(outPath, append(rendered, '\n'), 0o644); err != nil {
			fmt.Fprintf(stderr, "write report file: %v\n", err)
			return 1
		}
	}
	return code
}

func run(outPath, profile string) (report, int) {
	rep := report{}
	port, err := pickPort()
	if err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_setup", OK: false, Detail: err.Error()})
		rep.OK = false
		return rep, 1
	}
	operatorToken := "op_test_token"
	cfg := map[string]any{
		"security_profile": profile,
		"address":          fmt.Sprintf("127.0.0.1:%d", port),
		"audit_path":       filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-redteam-%d.jsonl", port)),
		"policy": map[string]any{
			"default_ttl_minutes":     5,
			"min_ttl_minutes":         1,
			"max_ttl_minutes":         30,
			"max_secrets_per_request": 5,
		},
		"auth": map[string]any{
			"enable_auth":                   true,
			"operator_token":                operatorToken,
			"allow_plaintext_secret_return": false,
			"session_ttl_minutes":           10,
			"grant_idle_timeout_minutes":    120,
			"grant_absolute_max_minutes":    240,
			"bootstrap_token_ttl_seconds":   60,
			"cleanup_interval_seconds":      60,
			"rate_limit_window_seconds":     60,
			"rate_limit_max_attempts":       100,
		},
		"execution_policy": map[string]any{
			"exact_match_executables": []string{"curl", "go", "python", "git", "npm", "node", "make", "pytest"},
			"denylist_substrings": []string{
				"printenv", "/proc/", "environ",
			},
			"output_security_mode": "none",
			"max_output_bytes":     32768,
			"default_timeout_sec":  10,
			"max_timeout_sec":      30,
		},
		"network_egress_policy": map[string]any{
			"enabled":              true,
			"require_intent_match": true,
			"allow_domains":        []string{"api.github.com"},
			"intent_allow_domains": map[string]any{"run_tests": []string{"api.github.com"}},
			"deny_substrings":      []string{"169.254.169.254", "metadata.google.internal", "localhost", "127.0.0.1"},
		},
		"secret_source": map[string]any{
			"type":                    "env",
			"env_prefix":              "PROMPTLOCK_SECRET_",
			"external_auth_token_env": "PROMPTLOCK_EXTERNAL_SECRET_TOKEN",
			"external_timeout_sec":    10,
			"in_memory_hardened":      "fail",
		},
		"intents": map[string]any{"run_tests": []string{"github_token"}},
	}

	td, err := os.MkdirTemp("", "promptlock-redteam-")
	if err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_setup", OK: false, Detail: err.Error()})
		rep.OK = false
		return rep, 1
	}
	defer os.RemoveAll(td)
	cfg["state_store_file"] = filepath.Join(td, "state-store.json")
	if authCfg, ok := cfg["auth"].(map[string]any); ok {
		authCfg["store_file"] = filepath.Join(td, "auth-store.json")
		authCfg["store_encryption_key_env"] = "PROMPTLOCK_AUTH_STORE_KEY"
	}

	transport, err := configureHarnessTransport(profile, td, port, cfg)
	if err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_setup", OK: false, Detail: err.Error()})
		rep.OK = false
		return rep, 1
	}

	cfgPath := filepath.Join(td, "config.json")
	cfgBytes, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_setup", OK: false, Detail: err.Error()})
		rep.OK = false
		return rep, 1
	}

	repo, err := os.Getwd()
	if err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_setup", OK: false, Detail: err.Error()})
		rep.OK = false
		return rep, 1
	}
	env := os.Environ()
	env = append(env, "PROMPTLOCK_CONFIG="+cfgPath)
	env = append(env, "PROMPTLOCK_SECRET_GITHUB_TOKEN=demo")
	env = append(env, "PROMPTLOCK_AUTH_STORE_KEY=redteam_auth_store_key_012345")
	if profile == "dev" {
		env = append(env, "PROMPTLOCK_ALLOW_DEV_PROFILE=1")
	}

	binPath := filepath.Join(td, "promptlockd-redteam")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/promptlockd")
	build.Dir = repo
	build.Env = env
	if out, err := build.CombinedOutput(); err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_build", OK: false, Detail: fmt.Sprintf("go build failed: %s", string(bytes.TrimSpace(out)))})
		rep.OK = false
		return rep, 1
	}

	logPath := filepath.Join(td, "broker.log")
	logf, err := os.Create(logPath)
	if err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_setup", OK: false, Detail: err.Error()})
		rep.OK = false
		return rep, 1
	}
	defer logf.Close()

	proc := exec.Command(binPath)
	proc.Dir = repo
	proc.Env = env
	proc.Stdout = logf
	proc.Stderr = logf
	if err := proc.Start(); err != nil {
		rep.Results = append(rep.Results, result{Name: "broker_start", OK: false, Detail: err.Error()})
		rep.OK = false
		return rep, 1
	}
	defer func() {
		_ = proc.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = proc.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = proc.Process.Kill()
			<-done
		}
	}()

	if !waitForUp(transport.operatorClient, transport.operatorBase, 45*time.Second) {
		rep.Results = append(rep.Results, result{Name: "broker_start", OK: false, Detail: "broker did not start in time", LogTail: tailLog(logPath, 20)})
		rep.OK = false
		return rep, 1
	}

	rep = runChecks(transport, operatorToken)
	if rep.OK {
		return rep, 0
	}
	return rep, 1
}

func runChecks(transport harnessTransport, operatorToken string) report {
	results := make([]result, 0, 10)
	add := func(r result) { results = append(results, r) }

	status, _, _ := httpJSON(transport.operatorClient, http.MethodPost, transport.operatorBase+"/v1/leases/approve?request_id=x", nil, nil)
	add(result{Name: "auth_bypass_operator_endpoint", OK: status == http.StatusUnauthorized, Status: status, Expected: http.StatusUnauthorized})

	opHeaders := map[string]string{"Authorization": "Bearer " + operatorToken}
	status, body, _ := httpJSON(transport.operatorClient, http.MethodPost, transport.operatorBase+"/v1/auth/bootstrap/create", map[string]any{"agent_id": "a1", "container_id": "c1"}, opHeaders)
	boot, _ := body["bootstrap_token"].(string)
	okBoot := status == http.StatusOK && boot != ""
	add(result{Name: "bootstrap_create", OK: okBoot, Status: status})
	if !okBoot {
		return finalize(results)
	}

	status, body, _ = httpJSON(transport.agentClient, http.MethodPost, transport.agentBase+"/v1/auth/pair/complete", map[string]any{"token": boot, "container_id": "c1"}, nil)
	grant, _ := body["grant_id"].(string)
	okPair := status == http.StatusOK && grant != ""
	add(result{Name: "pair_complete", OK: okPair, Status: status})
	if !okPair {
		return finalize(results)
	}

	status, _, _ = httpJSON(transport.agentClient, http.MethodPost, transport.agentBase+"/v1/auth/pair/complete", map[string]any{"token": boot, "container_id": "c1"}, nil)
	add(result{Name: "bootstrap_replay_denied", OK: status == http.StatusForbidden, Status: status, Expected: http.StatusForbidden})

	status, body, _ = httpJSON(transport.agentClient, http.MethodPost, transport.agentBase+"/v1/auth/session/mint", map[string]any{"grant_id": grant}, nil)
	sess, _ := body["session_token"].(string)
	okSess := status == http.StatusOK && sess != ""
	add(result{Name: "session_mint", OK: okSess, Status: status})
	if !okSess {
		return finalize(results)
	}

	agentHeaders := map[string]string{"Authorization": "Bearer " + sess}
	status, _, _ = httpJSON(transport.operatorClient, http.MethodPost, transport.operatorBase+"/v1/leases/approve?request_id=x", map[string]any{}, agentHeaders)
	add(result{Name: "role_confusion_agent_on_operator", OK: status == http.StatusUnauthorized, Status: status, Expected: http.StatusUnauthorized})

	leaseToken, leaseResults := createApprovedLeaseForEgressCheck(transport, agentHeaders, opHeaders)
	for _, r := range leaseResults {
		add(r)
		if !r.OK {
			return finalize(results)
		}
	}

	payload := map[string]any{
		"lease_token":         leaseToken,
		"intent":              "run_tests",
		"command":             []string{"curl", "http://169.254.169.254/latest/meta-data"},
		"secrets":             []string{"github_token"},
		"command_fingerprint": "fp",
		"workdir_fingerprint": "wd",
	}
	status, _, raw := httpJSON(transport.agentClient, http.MethodPost, transport.agentBase+"/v1/leases/execute", payload, agentHeaders)
	ok, detail := verifyNetworkEgressDeny(status, raw)
	add(result{Name: "egress_bypass_denied", OK: ok, Status: status, Expected: http.StatusForbidden, Detail: detail})

	return finalize(results)
}

func createApprovedLeaseForEgressCheck(transport harnessTransport, agentHeaders, opHeaders map[string]string) (string, []result) {
	results := make([]result, 0, 2)
	add := func(r result) { results = append(results, r) }

	requestPayload := map[string]any{
		"agent_id":            "a1",
		"task_id":             "redteam-egress-deny",
		"intent":              "run_tests",
		"reason":              "redteam live execute-time egress deny",
		"ttl_minutes":         5,
		"secrets":             []string{"github_token"},
		"command_fingerprint": "fp",
		"workdir_fingerprint": "wd",
	}
	status, body, _ := httpJSON(transport.agentClient, http.MethodPost, transport.agentBase+"/v1/leases/request", requestPayload, agentHeaders)
	requestID, _ := body["request_id"].(string)
	okRequest := status == http.StatusOK && requestID != ""
	add(result{Name: "lease_request_create", OK: okRequest, Status: status})
	if !okRequest {
		return "", results
	}

	status, body, _ = httpJSON(transport.operatorClient, http.MethodPost, transport.operatorBase+"/v1/leases/approve?request_id="+requestID, map[string]any{"ttl_minutes": 5}, opHeaders)
	leaseToken, _ := body["lease_token"].(string)
	okApprove := status == http.StatusOK && leaseToken != ""
	add(result{Name: "lease_request_approve", OK: okApprove, Status: status})
	if !okApprove {
		return "", results
	}

	return leaseToken, results
}

func finalize(results []result) report {
	ok := true
	for _, r := range results {
		if !r.OK {
			ok = false
			break
		}
	}
	return report{OK: ok, Results: results}
}

func verifyNetworkEgressDeny(status int, raw string) (bool, string) {
	detail := strings.TrimSpace(raw)
	if status != http.StatusForbidden {
		if detail == "" {
			detail = fmt.Sprintf("expected 403 with network egress-specific deny evidence, got %d", status)
		} else {
			detail = detail + fmt.Sprintf("; expected 403 with network egress-specific deny evidence, got %d", status)
		}
		return false, detail
	}
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "network egress") ||
		strings.Contains(lower, "inspectable destination") ||
		strings.Contains(lower, "not allowed by network policy") {
		return true, detail
	}
	if detail == "" {
		return false, "expected network egress-specific deny evidence"
	}
	return false, detail + "; expected network egress-specific deny evidence"
}

func configureHarnessTransport(profile, tempDir string, port int, cfg map[string]any) (harnessTransport, error) {
	delete(cfg, "tls")
	if profile == "hardened" {
		agentSocket := filepath.Join(tempDir, "promptlock-agent.sock")
		operatorSocket := filepath.Join(tempDir, "promptlock-operator.sock")
		cfg["agent_unix_socket"] = agentSocket
		cfg["operator_unix_socket"] = operatorSocket
		return harnessTransport{
			operatorClient: newUnixSocketClient(operatorSocket),
			operatorBase:   "http://unix",
			agentClient:    newUnixSocketClient(agentSocket),
			agentBase:      "http://unix",
		}, nil
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 5 * time.Second}
	return harnessTransport{
		operatorClient: client,
		operatorBase:   base,
		agentClient:    client,
		agentBase:      base,
	}, nil
}

func pickPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func waitForUp(client *http.Client, base string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, _, _ := httpJSON(client, http.MethodGet, base+"/v1/meta/capabilities", nil, nil)
		if status == http.StatusOK || status == http.StatusUnauthorized {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func newUnixSocketClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
}

func httpJSON(client *http.Client, method, url string, body any, headers map[string]string) (int, map[string]any, string) {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, nil, ""
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, ""
	}
	defer resp.Body.Close()
	rawBytes, _ := io.ReadAll(resp.Body)
	raw := string(rawBytes)
	parsed := map[string]any{}
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &parsed)
	}
	return resp.StatusCode, parsed, raw
}

func tailLog(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := bytes.Split(b, []byte{'\n'})
	if len(lines) <= n {
		return string(bytes.TrimSpace(b))
	}
	return string(bytes.Join(lines[len(lines)-n:], []byte{'\n'}))
}
