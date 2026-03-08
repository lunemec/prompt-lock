package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id,omitempty"`
	Result  any         `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type callParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type execArgs struct {
	Intent string
	Cmd    []string
	TTL    int
}

const maxRPCLineBytes = 1 << 20 // 1 MiB

func main() {
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 0, 64*1024), maxRPCLineBytes)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		var req rpcReq
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(rpcResp{JSONRPC: "2.0", Error: map[string]any{"code": -32700, "message": err.Error()}})
			continue
		}
		handle(req)
	}
	if err := s.Err(); err != nil {
		emit(rpcResp{JSONRPC: "2.0", Error: map[string]any{"code": -32001, "message": "stdin scanner error: " + err.Error()}})
	}
}

func handle(req rpcReq) {
	switch req.Method {
	case "initialize":
		emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"serverInfo": map[string]string{"name": "promptlock-mcp", "version": "0.1.0"}}})
	case "tools/list":
		emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": []map[string]any{{
			"name":        "execute_with_intent",
			"description": "Request lease by intent and execute command via broker-exec path.",
			"inputSchema": map[string]any{"type": "object"},
		}}}})
	case "tools/call":
		var p callParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32602, "message": err.Error()}})
			return
		}
		if p.Name != "execute_with_intent" {
			emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32601, "message": "unknown tool"}})
			return
		}
		out, err := executeWithIntent(p.Arguments)
		if err != nil {
			emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32000, "message": err.Error()}})
			return
		}
		emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"content": []map[string]string{{"type": "text", "text": out}}}})
	default:
		emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32601, "message": "method not found"}})
	}
}

func emit(v rpcResp) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func executeWithIntent(args map[string]interface{}) (string, error) {
	broker := envDefault("PROMPTLOCK_BROKER_URL", "http://127.0.0.1:8765")
	session := os.Getenv("PROMPTLOCK_SESSION_TOKEN")
	if session == "" {
		return "", fmt.Errorf("PROMPTLOCK_SESSION_TOKEN is required")
	}
	validated, err := parseAndValidateExecArgs(args)
	if err != nil {
		return "", err
	}
	intent := validated.Intent
	cmd := validated.Cmd
	ttl := validated.TTL

	// resolve intent
	var resolved struct {
		Secrets []string `json:"secrets"`
	}
	if err := postAuth(broker+"/v1/intents/resolve", session, map[string]any{"intent": intent}, &resolved); err != nil {
		return "", err
	}

	fp := sha(cmd)
	wdfp := sha([]string{cwd()})

	// request lease
	var reqOut struct {
		RequestID string `json:"request_id"`
	}
	if err := postAuth(broker+"/v1/leases/request", session, map[string]any{
		"agent_id":            envDefault("PROMPTLOCK_AGENT_ID", "mcp-agent"),
		"task_id":             envDefault("PROMPTLOCK_TASK_ID", "mcp-task"),
		"reason":              "mcp execute_with_intent",
		"ttl_minutes":         ttl,
		"secrets":             resolved.Secrets,
		"command_fingerprint": fp,
		"workdir_fingerprint": wdfp,
	}, &reqOut); err != nil {
		return "", err
	}

	// wait approval
	approvalTimeoutSec := envIntDefault("PROMPTLOCK_APPROVAL_TIMEOUT_SEC", 120)
	if approvalTimeoutSec < 1 {
		approvalTimeoutSec = 1
	}
	deadline := time.Now().Add(time.Duration(approvalTimeoutSec) * time.Second)
	for {
		var st struct {
			Status string `json:"status"`
		}
		if err := getAuth(broker+"/v1/requests/status?request_id="+reqOut.RequestID, session, &st); err != nil {
			return "", err
		}
		if st.Status == "denied" {
			return "", fmt.Errorf("request denied")
		}
		if st.Status == "approved" {
			break
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("approval timeout")
		}
		time.Sleep(2 * time.Second)
	}

	var lease struct {
		LeaseToken string `json:"lease_token"`
	}
	if err := getAuth(broker+"/v1/leases/by-request?request_id="+reqOut.RequestID, session, &lease); err != nil {
		return "", err
	}

	var execOut struct {
		ExitCode     int    `json:"exit_code"`
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := postAuth(broker+"/v1/leases/execute", session, map[string]any{
		"lease_token":         lease.LeaseToken,
		"command":             cmd,
		"secrets":             resolved.Secrets,
		"command_fingerprint": fp,
		"workdir_fingerprint": wdfp,
	}, &execOut); err != nil {
		return "", err
	}
	return fmt.Sprintf("exit=%d\n%s", execOut.ExitCode, execOut.StdoutStderr), nil
}

func postAuth(url, token string, in any, out any) error {
	b, _ := json.Marshal(in)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getAuth(url, token string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func envDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envIntDefault(k string, d int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func parseAndValidateExecArgs(m map[string]interface{}) (execArgs, error) {
	intent, _ := m["intent"].(string)
	intent = strings.TrimSpace(intent)
	if intent == "" || len(intent) > 64 {
		return execArgs{}, fmt.Errorf("intent is required (1..64 chars)")
	}
	for _, r := range intent {
		if !(r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return execArgs{}, fmt.Errorf("intent contains invalid characters")
		}
	}
	cmdAny, ok := m["command"].([]interface{})
	if !ok || len(cmdAny) == 0 || len(cmdAny) > 32 {
		return execArgs{}, fmt.Errorf("command array is required (1..32 parts)")
	}
	cmd := make([]string, 0, len(cmdAny))
	for _, x := range cmdAny {
		s := strings.TrimSpace(fmt.Sprint(x))
		if s == "" || len(s) > 256 || strings.ContainsAny(s, "\r\n") {
			return execArgs{}, fmt.Errorf("invalid command argument")
		}
		cmd = append(cmd, s)
	}
	ttl := 5
	if v, ok := m["ttl_minutes"].(float64); ok {
		ttl = int(v)
	}
	if ttl < 1 || ttl > 60 {
		return execArgs{}, fmt.Errorf("ttl_minutes out of range (1..60)")
	}
	return execArgs{Intent: intent, Cmd: cmd, TTL: ttl}, nil
}

func cwd() string { wd, _ := os.Getwd(); return wd }
func sha(parts []string) string {
	s := strings.Join(parts, "\x00")
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
