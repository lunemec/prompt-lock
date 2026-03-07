package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
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

func main() {
	if len(os.Args) < 2 || os.Args[1] != "exec" {
		fmt.Fprintln(os.Stderr, "usage: promptlock exec [flags] -- <command>")
		os.Exit(2)
	}

	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	broker := fs.String("broker", getenv("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765"), "broker URL")
	agent := fs.String("agent", "agent", "agent id")
	task := fs.String("task", "task", "task id")
	reason := fs.String("reason", "execute command", "reason")
	ttl := fs.Int("ttl", 5, "ttl minutes")
	intent := fs.String("intent", "", "intent name")
	secretsCSV := fs.String("secrets", "", "comma-separated secret names")
	autoApprove := fs.Bool("auto-approve", false, "approve immediately (demo only; requires PROMPTLOCK_DEV_MODE=1)")
	waitApprove := fs.Duration("wait-approve", 2*time.Minute, "max time to wait for external approval")
	pollInterval := fs.Duration("poll-interval", 2*time.Second, "poll interval while waiting for approval")
	allowRisky := fs.Bool("allow-risky-command", false, "allow risky commands (env/printenv/proc environ reads)")
	fs.Parse(os.Args[2:])

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
		resolved, err := resolveIntent(*broker, *intent)
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

	fingerprint := commandFingerprint(cmdArgs)
	wdfp, err := workdirFingerprint()
	if err != nil {
		fatal(err)
	}
	reqID, err := requestLease(*broker, requestBody{AgentID: *agent, TaskID: *task, Reason: *reason, TTLMinutes: *ttl, Secrets: secrets, CommandFingerprint: fingerprint, WorkdirFingerprint: wdfp})
	if err != nil {
		fatal(err)
	}

	var lease string
	if *autoApprove {
		if getenv("PROMPTLOCK_DEV_MODE", "") != "1" {
			fatal(fmt.Errorf("--auto-approve is disabled unless PROMPTLOCK_DEV_MODE=1"))
		}
		lease, err = approve(*broker, reqID, *ttl)
		if err != nil {
			fatal(err)
		}
	} else {
		lease, err = waitForApproval(*broker, reqID, *waitApprove, *pollInterval)
		if err != nil {
			fatal(err)
		}
	}

	env := os.Environ()
	for _, s := range secrets {
		v, err := accessSecret(*broker, lease, s, fingerprint, wdfp)
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

func resolveIntent(broker, intent string) ([]string, error) {
	var out struct {
		Secrets []string `json:"secrets"`
	}
	if err := postJSON(broker+"/v1/intents/resolve", map[string]string{"intent": intent}, &out); err != nil {
		return nil, err
	}
	return out.Secrets, nil
}

func requestLease(broker string, req requestBody) (string, error) {
	var out struct {
		RequestID string `json:"request_id"`
	}
	if err := postJSON(broker+"/v1/leases/request", req, &out); err != nil {
		return "", err
	}
	if out.RequestID == "" {
		return "", fmt.Errorf("empty request_id")
	}
	return out.RequestID, nil
}

func approve(broker, requestID string, ttl int) (string, error) {
	var out struct {
		LeaseToken string `json:"lease_token"`
	}
	if err := postJSON(broker+"/v1/leases/approve?request_id="+requestID, map[string]int{"ttl_minutes": ttl}, &out); err != nil {
		return "", err
	}
	if out.LeaseToken == "" {
		return "", fmt.Errorf("empty lease_token")
	}
	return out.LeaseToken, nil
}

func accessSecret(broker, lease, secret, fingerprint, workdirFP string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	if err := postJSON(broker+"/v1/leases/access", map[string]string{"lease_token": lease, "secret": secret, "command_fingerprint": fingerprint, "workdir_fingerprint": workdirFP}, &out); err != nil {
		return "", err
	}
	return out.Value, nil
}

func postJSON(url string, in any, out any) error {
	b, _ := json.Marshal(in)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
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

func fetchLeaseByRequest(broker, requestID string) (string, error) {
	resp, err := http.Get(broker + "/v1/leases/by-request?request_id=" + requestID)
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

func requestStatus(broker, requestID string) (string, error) {
	resp, err := http.Get(broker + "/v1/requests/status?request_id=" + requestID)
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

func waitForApproval(broker, requestID string, timeout, poll time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		status, err := requestStatus(broker, requestID)
		if err != nil {
			return "", err
		}
		switch status {
		case "approved":
			return fetchLeaseByRequest(broker, requestID)
		case "denied":
			return "", fmt.Errorf("request denied: %s", requestID)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("approval timeout for request %s", requestID)
		}
		time.Sleep(poll)
	}
}
