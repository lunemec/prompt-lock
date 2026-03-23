package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lunemec/promptlock/internal/buildinfo"
	"github.com/lunemec/promptlock/internal/config"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id"`
	Result  any         `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type callParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type execArgs struct {
	Intent  string
	Cmd     []string
	TTL     int
	EnvPath string
}

type brokerTransport struct {
	BaseURL    string
	UnixSocket string
}

type envLookup func(string) string
type pathStat func(string) error

const maxRPCLineBytes = 1 << 20 // 1 MiB
const mcpProtocolVersion = "2024-11-05"
const executeWithIntentToolDescription = "Request a lease by configured intent id and execute command via broker-exec path. intent must be a configured intent id (for example run_tests), not free-form text."
const unknownIntentHint = "intent must be a configured intent id (for example run_tests); check broker config.intents or your quickstart setup"

var brokerClientTimeout = 10 * time.Second
var version string

type mcpServer struct {
	emitMu     sync.Mutex
	inflightMu sync.Mutex
	inflight   map[string]context.CancelFunc
}

func newMCPServer() *mcpServer {
	return &mcpServer{
		inflight: map[string]context.CancelFunc{},
	}
}

func main() {
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 0, 64*1024), maxRPCLineBytes)
	server := newMCPServer()
	defer server.cancelAllInFlight()
	shouldExit := false
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			server.emit(rpcResp{JSONRPC: "2.0", Error: map[string]any{"code": -32600, "message": "batch requests are not supported"}})
			continue
		}
		var req rpcReq
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			server.emit(rpcResp{JSONRPC: "2.0", Error: map[string]any{"code": -32700, "message": err.Error()}})
			continue
		}
		if !isValidRPCRequestID(req.ID) {
			server.emit(rpcResp{JSONRPC: "2.0", ID: nil, Error: map[string]any{"code": -32600, "message": "invalid request id"}})
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			server.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32600, "message": "invalid request"}})
			continue
		}
		if server.handle(req) {
			shouldExit = true
			break
		}
	}
	if shouldExit {
		return
	}
	if err := s.Err(); err != nil {
		server.emit(rpcResp{JSONRPC: "2.0", Error: map[string]any{"code": -32001, "message": "stdin scanner error: " + err.Error()}})
	}
}

func (s *mcpServer) handle(req rpcReq) bool {
	notify := req.ID == nil
	switch req.Method {
	case "ping":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
		}
	case "initialized", "notifications/initialized":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
		}
	case "notifications/cancelled":
		s.cancelInFlightFromParams(req.Params)
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
		}
	case "shutdown":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
		}
	case "exit":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
		}
		return true
	case "initialize":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
				"protocolVersion": selectProtocolVersion(req.Params),
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]string{"name": "promptlock-mcp", "version": buildinfo.ResolveVersion(version)},
			}})
		}
	case "tools/list":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": []map[string]any{{
				"name":        "execute_with_intent",
				"description": executeWithIntentToolDescription,
				"inputSchema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"intent", "command"},
					"properties": map[string]any{
						"intent": map[string]any{
							"type":        "string",
							"minLength":   1,
							"maxLength":   64,
							"pattern":     "^[A-Za-z0-9_-]+$",
							"description": "Broker intent identifier; must exactly match a configured intent id. Quickstart usually uses run_tests.",
						},
						"command": map[string]any{
							"type":        "array",
							"minItems":    1,
							"maxItems":    32,
							"description": "Command argv to execute (first item is executable, remaining items are args).",
							"items": map[string]any{
								"type":      "string",
								"minLength": 1,
								"maxLength": 256,
							},
						},
						"ttl_minutes": map[string]any{
							"type":        "integer",
							"minimum":     1,
							"maximum":     60,
							"default":     5,
							"description": "Optional lease TTL in minutes (defaults to 5).",
						},
						"env_path": map[string]any{
							"type":        "string",
							"minLength":   1,
							"maxLength":   4096,
							"pattern":     "^[^\r\n]+$",
							"description": "Optional single-line .env path for approval context (example: demo-envs/github.env).",
						},
					},
					"examples": []map[string]any{{
						"intent":      "run_tests",
						"command":     []string{"make", "demo-print-github-token"},
						"env_path":    "demo-envs/github.env",
						"ttl_minutes": 5,
					}},
				},
			}}}})
		}
	case "resources/list":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resources": []any{}}})
		}
	case "prompts/list":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"prompts": []any{}}})
		}
	case "tools/call":
		if notify {
			return false
		}
		if isNullOrEmptyParams(req.Params) {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32602, "message": "invalid params: tools/call params object is required"}})
			return false
		}
		var p callParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32602, "message": err.Error()}})
			return false
		}
		if strings.TrimSpace(p.Name) == "" {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32602, "message": "invalid params: tools/call.name is required"}})
			return false
		}
		if p.Name != "execute_with_intent" {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32601, "message": "unknown tool"}})
			return false
		}
		reqKey, ok := rpcRequestKey(req.ID)
		if !ok {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32600, "message": "invalid request id"}})
			return false
		}
		ctx, cancel := context.WithCancel(context.Background())
		if !s.registerInFlight(reqKey, cancel) {
			cancel()
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32600, "message": "duplicate in-flight request id"}})
			return false
		}
		go func(id any, reqKey string, args map[string]interface{}) {
			defer s.unregisterInFlight(reqKey)
			out, err := executeWithIntent(ctx, args)
			if err != nil {
				s.emit(rpcResp{JSONRPC: "2.0", ID: id, Error: map[string]any{"code": -32000, "message": err.Error()}})
				return
			}
			s.emit(rpcResp{JSONRPC: "2.0", ID: id, Result: map[string]any{"content": []map[string]string{{"type": "text", "text": out}}}})
		}(req.ID, reqKey, p.Arguments)
	default:
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32601, "message": "method not found"}})
		}
	}
	return false
}

func (s *mcpServer) emit(v rpcResp) {
	b, _ := json.Marshal(v)
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	fmt.Println(string(b))
}

func (s *mcpServer) registerInFlight(reqKey string, cancel context.CancelFunc) bool {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if _, exists := s.inflight[reqKey]; exists {
		return false
	}
	s.inflight[reqKey] = cancel
	return true
}

func (s *mcpServer) unregisterInFlight(reqKey string) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	delete(s.inflight, reqKey)
}

func (s *mcpServer) cancelInFlightFromParams(params json.RawMessage) {
	reqID, ok := parseCancellationRequestID(params)
	if !ok {
		return
	}
	reqKey, ok := rpcRequestKey(reqID)
	if !ok {
		return
	}
	s.inflightMu.Lock()
	cancel, ok := s.inflight[reqKey]
	s.inflightMu.Unlock()
	if ok {
		cancel()
	}
}

func (s *mcpServer) cancelAllInFlight() {
	s.inflightMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.inflight))
	for _, cancel := range s.inflight {
		cancels = append(cancels, cancel)
	}
	s.inflightMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func parseCancellationRequestID(params json.RawMessage) (any, bool) {
	if isNullOrEmptyParams(params) {
		return nil, false
	}
	var in map[string]any
	if err := json.Unmarshal(params, &in); err != nil {
		return nil, false
	}
	if reqID, ok := in["requestId"]; ok {
		if !isValidRPCRequestID(reqID) || reqID == nil {
			return nil, false
		}
		return reqID, true
	}
	if reqID, ok := in["id"]; ok {
		if !isValidRPCRequestID(reqID) || reqID == nil {
			return nil, false
		}
		return reqID, true
	}
	return nil, false
}

func rpcRequestKey(id any) (string, bool) {
	switch v := id.(type) {
	case nil:
		return "", false
	case string:
		if strings.TrimSpace(v) == "" {
			return "", false
		}
		return "str:" + v, true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", false
		}
		if math.Trunc(v) == v {
			return fmt.Sprintf("num:%d", int64(v)), true
		}
		return fmt.Sprintf("num:%g", v), true
	default:
		return "", false
	}
}

func isValidRPCRequestID(id any) bool {
	if id == nil {
		return true
	}
	_, ok := rpcRequestKey(id)
	return ok
}

func selectProtocolVersion(params json.RawMessage) string {
	if isNullOrEmptyParams(params) {
		return mcpProtocolVersion
	}
	var in struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		return mcpProtocolVersion
	}
	if strings.TrimSpace(in.ProtocolVersion) == mcpProtocolVersion {
		return mcpProtocolVersion
	}
	return mcpProtocolVersion
}

func isNullOrEmptyParams(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func executeWithIntent(ctx context.Context, args map[string]interface{}) (string, error) {
	broker, err := resolveBrokerTransport(os.Getenv, statPath)
	if err != nil {
		return "", err
	}
	session := effectiveEnv(os.Getenv, "PROMPTLOCK_SESSION_TOKEN")
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
	envPath := validated.EnvPath

	// resolve intent
	var resolved struct {
		Secrets []string `json:"secrets"`
	}
	if err := postAuth(ctx, broker, "/v1/intents/resolve", session, map[string]any{"intent": intent}, &resolved); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request cancelled")
		}
		if isUnknownIntentError(err) {
			return "", fmt.Errorf("%w: %s", err, unknownIntentHint)
		}
		return "", err
	}

	fp := sha(cmd)
	wdfp := sha([]string{cwd()})

	// request lease
	var reqOut struct {
		RequestID string `json:"request_id"`
	}
	requestBody := map[string]any{
		"agent_id":            envDefault("PROMPTLOCK_AGENT_ID", "mcp-agent"),
		"task_id":             envDefault("PROMPTLOCK_TASK_ID", "mcp-task"),
		"intent":              intent,
		"reason":              "mcp execute_with_intent",
		"ttl_minutes":         ttl,
		"secrets":             resolved.Secrets,
		"command_fingerprint": fp,
		"workdir_fingerprint": wdfp,
	}
	if envPath != "" {
		requestBody["env_path"] = envPath
	}
	if err := postAuth(ctx, broker, "/v1/leases/request", session, requestBody, &reqOut); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request cancelled")
		}
		return "", err
	}
	requestID := strings.TrimSpace(reqOut.RequestID)
	approved := false
	defer func() {
		if ctx.Err() == nil {
			return
		}
		if approved {
			return
		}
		if requestID == "" {
			return
		}
		if err := cancelPendingRequest(broker, session, requestID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to cancel pending request %s during MCP cancellation cleanup: %v\n", requestID, err)
		}
	}()

	// wait approval
	approvalTimeoutSec := envIntDefault("PROMPTLOCK_APPROVAL_TIMEOUT_SEC", 120)
	if approvalTimeoutSec < 1 {
		approvalTimeoutSec = 1
	}
	deadline := time.Now().Add(time.Duration(approvalTimeoutSec) * time.Second)
	for {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request cancelled")
		}
		var st struct {
			Status string `json:"status"`
		}
		if err := getAuth(ctx, broker, "/v1/requests/status?request_id="+reqOut.RequestID, session, &st); err != nil {
			if ctx.Err() != nil {
				return "", fmt.Errorf("request cancelled")
			}
			return "", err
		}
		if st.Status == "denied" {
			return "", fmt.Errorf("request denied")
		}
		if st.Status == "approved" {
			approved = true
			break
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("approval timeout")
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("request cancelled")
		case <-time.After(2 * time.Second):
		}
	}

	var lease struct {
		LeaseToken string `json:"lease_token"`
	}
	if err := getAuth(ctx, broker, "/v1/leases/by-request?request_id="+reqOut.RequestID, session, &lease); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request cancelled")
		}
		return "", err
	}

	var execOut struct {
		ExitCode     int    `json:"exit_code"`
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := postAuth(ctx, broker, "/v1/leases/execute", session, map[string]any{
		"lease_token":         lease.LeaseToken,
		"intent":              intent,
		"command":             cmd,
		"secrets":             resolved.Secrets,
		"command_fingerprint": fp,
		"workdir_fingerprint": wdfp,
	}, &execOut); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request cancelled")
		}
		return "", err
	}
	return fmt.Sprintf("exit=%d\n%s", execOut.ExitCode, execOut.StdoutStderr), nil
}

func cancelPendingRequest(broker brokerTransport, session, requestID string) error {
	cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var out map[string]any
	return postAuth(cancelCtx, broker, "/v1/leases/cancel?request_id="+neturl.QueryEscape(requestID), session, map[string]any{"reason": "mcp notification cancelled"}, &out)
}

func postAuth(ctx context.Context, broker brokerTransport, path, token string, in any, out any) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brokerRequestURL(broker, path), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client, err := brokerHTTPClient(broker)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return normalizeBrokerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return brokerResponseError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getAuth(ctx context.Context, broker brokerTransport, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, brokerRequestURL(broker, path), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client, err := brokerHTTPClient(broker)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return normalizeBrokerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return brokerResponseError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func brokerResponseError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	msg := strings.Join(strings.Fields(string(body)), " ")
	if msg == "" {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return fmt.Errorf("request failed: %s: %s", resp.Status, msg)
}

func envDefault(k, d string) string {
	if v := effectiveEnv(os.Getenv, k); v != "" {
		return v
	}
	return d
}

func effectiveEnv(getenv envLookup, key string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
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
	case "PROMPTLOCK_AGENT_ID":
		if v := strings.TrimSpace(getenv("PROMPTLOCK_AGENT_ID")); v != "" {
			return v
		}
		return strings.TrimSpace(getenv("PROMPTLOCK_WRAPPER_AGENT_ID"))
	case "PROMPTLOCK_TASK_ID":
		if v := strings.TrimSpace(getenv("PROMPTLOCK_TASK_ID")); v != "" {
			return v
		}
		return strings.TrimSpace(getenv("PROMPTLOCK_WRAPPER_TASK_ID"))
	}
	return strings.TrimSpace(getenv(key))
}

func lookupEnvMap(values map[string]string) envLookup {
	return func(key string) string {
		if values == nil {
			return ""
		}
		return values[key]
	}
}

func statPath(path string) error {
	_, err := os.Stat(path)
	return err
}

func resolveBrokerTransport(getenv envLookup, stat pathStat) (brokerTransport, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	explicitUnix := strings.TrimSpace(getenv("PROMPTLOCK_BROKER_UNIX_SOCKET"))
	if explicitUnix != "" {
		if stat != nil {
			if err := stat(explicitUnix); err != nil {
				return brokerTransport{}, fmt.Errorf("broker unix socket not found at %s", explicitUnix)
			}
		}
		return brokerTransport{UnixSocket: explicitUnix}, nil
	}
	agentUnix := effectiveEnv(getenv, "PROMPTLOCK_AGENT_UNIX_SOCKET")
	if agentUnix == "" {
		agentUnix = config.DefaultAgentUnixSocketPath
	}
	if stat != nil {
		if err := stat(agentUnix); err == nil {
			return brokerTransport{UnixSocket: agentUnix}, nil
		}
	}
	explicitURL := effectiveEnv(getenv, "PROMPTLOCK_BROKER_URL")
	if explicitURL != "" {
		return brokerTransport{BaseURL: explicitURL}, nil
	}
	if stat != nil {
		if err := stat(agentUnix); err != nil {
			return brokerTransport{}, fmt.Errorf("agent broker unix socket not found at %s; set PROMPTLOCK_BROKER_URL for explicit TCP transport", agentUnix)
		}
	}
	return brokerTransport{UnixSocket: agentUnix}, nil
}

func brokerRequestURL(broker brokerTransport, path string) string {
	base := strings.TrimSpace(broker.BaseURL)
	if strings.TrimSpace(broker.UnixSocket) != "" {
		base = "http://promptlock"
	}
	if strings.HasSuffix(base, "/") && strings.HasPrefix(path, "/") {
		return strings.TrimSuffix(base, "/") + path
	}
	if !strings.HasSuffix(base, "/") && !strings.HasPrefix(path, "/") {
		return base + "/" + path
	}
	return base + path
}

func brokerHTTPClient(broker brokerTransport) (*http.Client, error) {
	if strings.TrimSpace(broker.UnixSocket) == "" {
		return &http.Client{Timeout: brokerClientTimeout}, nil
	}
	socketPath := strings.TrimSpace(broker.UnixSocket)
	return &http.Client{
		Timeout: brokerClientTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}, nil
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
			return execArgs{}, fmt.Errorf("intent contains invalid characters (allowed: A-Z a-z 0-9 _ -)")
		}
	}
	cmdAny, ok := m["command"].([]interface{})
	if !ok || len(cmdAny) == 0 || len(cmdAny) > 32 {
		return execArgs{}, fmt.Errorf("command array is required (1..32 parts)")
	}
	cmd := make([]string, 0, len(cmdAny))
	for _, x := range cmdAny {
		raw, ok := x.(string)
		if !ok {
			return execArgs{}, fmt.Errorf("invalid command argument")
		}
		s := strings.TrimSpace(raw)
		if s == "" || len(s) > 256 || strings.ContainsAny(s, "\r\n") {
			return execArgs{}, fmt.Errorf("invalid command argument")
		}
		cmd = append(cmd, s)
	}
	ttl := 5
	if raw, ok := m["ttl_minutes"]; ok {
		v, ok := raw.(float64)
		if !ok || math.Trunc(v) != v {
			return execArgs{}, fmt.Errorf("ttl_minutes out of range (1..60)")
		}
		ttl = int(v)
	}
	if ttl < 1 || ttl > 60 {
		return execArgs{}, fmt.Errorf("ttl_minutes out of range (1..60)")
	}
	envPath := ""
	if raw, ok := m["env_path"]; ok {
		v, ok := raw.(string)
		if !ok {
			return execArgs{}, fmt.Errorf("env_path must be a string")
		}
		envPath = strings.TrimSpace(v)
		if envPath == "" || len(envPath) > 4096 || strings.ContainsAny(envPath, "\r\n") {
			return execArgs{}, fmt.Errorf("env_path must be a single-line path (1..4096 chars)")
		}
	}
	return execArgs{Intent: intent, Cmd: cmd, TTL: ttl, EnvPath: envPath}, nil
}

func cwd() string { wd, _ := os.Getwd(); return wd }
func sha(parts []string) string {
	s := strings.Join(parts, "\x00")
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func isUnknownIntentError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unknown intent")
}
