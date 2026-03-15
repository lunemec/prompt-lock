package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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
	Intent string
	Cmd    []string
	TTL    int
}

type brokerTransport struct {
	BaseURL    string
	UnixSocket string
}

type envLookup func(string) string
type pathStat func(string) error

const maxRPCLineBytes = 1 << 20 // 1 MiB
const mcpProtocolVersion = "2024-11-05"

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
				"serverInfo": map[string]string{"name": "promptlock-mcp", "version": "0.1.0"},
			}})
		}
	case "tools/list":
		if !notify {
			s.emit(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": []map[string]any{{
				"name":        "execute_with_intent",
				"description": "Request lease by intent and execute command via broker-exec path.",
				"inputSchema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"intent", "command"},
					"properties": map[string]any{
						"intent": map[string]any{
							"type":      "string",
							"minLength": 1,
							"maxLength": 64,
							"pattern":   "^[A-Za-z0-9_-]+$",
						},
						"command": map[string]any{
							"type":     "array",
							"minItems": 1,
							"maxItems": 32,
							"items": map[string]any{
								"type":      "string",
								"minLength": 1,
								"maxLength": 256,
							},
						},
						"ttl_minutes": map[string]any{
							"type":    "integer",
							"minimum": 1,
							"maximum": 60,
						},
					},
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
	if err := postAuth(ctx, broker, "/v1/intents/resolve", session, map[string]any{"intent": intent}, &resolved); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request cancelled")
		}
		return "", err
	}

	fp := sha(cmd)
	wdfp := sha([]string{cwd()})

	// request lease
	var reqOut struct {
		RequestID string `json:"request_id"`
	}
	if err := postAuth(ctx, broker, "/v1/leases/request", session, map[string]any{
		"agent_id":            envDefault("PROMPTLOCK_AGENT_ID", "mcp-agent"),
		"task_id":             envDefault("PROMPTLOCK_TASK_ID", "mcp-task"),
		"reason":              "mcp execute_with_intent",
		"ttl_minutes":         ttl,
		"secrets":             resolved.Secrets,
		"command_fingerprint": fp,
		"workdir_fingerprint": wdfp,
	}, &reqOut); err != nil {
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
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, brokerRequestURL(broker, path), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client, err := brokerHTTPClient(broker)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getAuth(ctx context.Context, broker brokerTransport, path, token string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, brokerRequestURL(broker, path), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	client, err := brokerHTTPClient(broker)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
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
	explicitURL := strings.TrimSpace(getenv("PROMPTLOCK_BROKER_URL"))
	if explicitURL != "" {
		return brokerTransport{BaseURL: explicitURL}, nil
	}
	agentUnix := strings.TrimSpace(getenv("PROMPTLOCK_AGENT_UNIX_SOCKET"))
	if agentUnix == "" {
		agentUnix = config.DefaultAgentUnixSocketPath
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
		return http.DefaultClient, nil
	}
	socketPath := strings.TrimSpace(broker.UnixSocket)
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}, nil
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
	return execArgs{Intent: intent, Cmd: cmd, TTL: ttl}, nil
}

func cwd() string { wd, _ := os.Getwd(); return wd }
func sha(parts []string) string {
	s := strings.Join(parts, "\x00")
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
