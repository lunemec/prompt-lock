package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/envpath"
	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestPromptLockV1IntegrationSecurityRegression(t *testing.T) {
	now := time.Date(2026, time.March, 12, 7, 15, 0, 0, time.UTC)

	root := t.TempDir()
	envFile := filepath.Join(root, ".env")
	if err := os.WriteFile(envFile, []byte("github_token=from_dotenv\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	outsideFile := filepath.Join(filepath.Dir(root), "outside.env")
	if err := os.WriteFile(outsideFile, []byte("github_token=outside_root\n"), 0o600); err != nil {
		t.Fatalf("write outside env file: %v", err)
	}

	envStore, err := envpath.New(root)
	if err != nil {
		t.Fatalf("new env-path store: %v", err)
	}

	store := memory.NewStore()
	store.SetSecret("github_token", "from_primary_store")
	store.SetSecret("npm_token", "from_primary_store_npm")

	audit := &captureAudit{}
	requestSeq := 0
	leaseSeq := 0

	s := &server{
		svc: app.Service{
			Policy:         domain.DefaultPolicy(),
			Requests:       store,
			Leases:         store,
			Secrets:        store,
			EnvPathSecrets: envStore,
			Audit:          audit,
			Now:            func() time.Time { return now },
			NewRequestID: func() string {
				requestSeq++
				return fmt.Sprintf("req-v1-%d", requestSeq)
			},
			NewLeaseTok: func() string {
				leaseSeq++
				return fmt.Sprintf("lease-v1-%d", leaseSeq)
			},
		},
		authEnabled: true,
		authCfg: config.AuthConfig{
			EnableAuth:                 true,
			OperatorToken:              "operator-token",
			AllowPlaintextSecretReturn: false,
			SessionTTLMinutes:          10,
			GrantIdleTimeoutMinutes:    480,
			GrantAbsoluteMaxMinutes:    10080,
			BootstrapTokenTTLSeconds:   60,
		},
		authStore: auth.NewStore(),
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"bash"},
			DenylistSubstrings:    []string{"printenv"},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		now: func() time.Time { return now },
	}

	bootstrapW := callJSONHandler(t, s.handleAuthBootstrapCreate, http.MethodPost, "/v1/auth/bootstrap/create", "operator-token", `{"agent_id":"agent-1","container_id":"container-1"}`)
	if bootstrapW.Code != http.StatusOK {
		t.Fatalf("bootstrap create failed: code=%d body=%s", bootstrapW.Code, bootstrapW.Body.String())
	}
	var bootstrapResp struct {
		BootstrapToken string `json:"bootstrap_token"`
	}
	decodeJSONBody(t, bootstrapW, &bootstrapResp)
	if strings.TrimSpace(bootstrapResp.BootstrapToken) == "" {
		t.Fatalf("expected non-empty bootstrap token")
	}

	pairW := callJSONHandler(t, s.handleAuthPairComplete, http.MethodPost, "/v1/auth/pair/complete", "", fmt.Sprintf(`{"token":%q,"container_id":"container-1"}`, bootstrapResp.BootstrapToken))
	if pairW.Code != http.StatusOK {
		t.Fatalf("pair complete failed: code=%d body=%s", pairW.Code, pairW.Body.String())
	}
	var pairResp struct {
		GrantID string `json:"grant_id"`
	}
	decodeJSONBody(t, pairW, &pairResp)
	if strings.TrimSpace(pairResp.GrantID) == "" {
		t.Fatalf("expected non-empty grant id")
	}

	mintW := callJSONHandler(t, s.handleAuthSessionMint, http.MethodPost, "/v1/auth/session/mint", "", fmt.Sprintf(`{"grant_id":%q}`, pairResp.GrantID))
	if mintW.Code != http.StatusOK {
		t.Fatalf("session mint failed: code=%d body=%s", mintW.Code, mintW.Body.String())
	}
	var mintResp struct {
		SessionToken string `json:"session_token"`
	}
	decodeJSONBody(t, mintW, &mintResp)
	if strings.TrimSpace(mintResp.SessionToken) == "" {
		t.Fatalf("expected non-empty session token")
	}

	mintSession := func(agentID, containerID string) string {
		t.Helper()
		bootstrapW := callJSONHandler(t, s.handleAuthBootstrapCreate, http.MethodPost, "/v1/auth/bootstrap/create", "operator-token", fmt.Sprintf(`{"agent_id":%q,"container_id":%q}`, agentID, containerID))
		if bootstrapW.Code != http.StatusOK {
			t.Fatalf("bootstrap create failed for %s: code=%d body=%s", agentID, bootstrapW.Code, bootstrapW.Body.String())
		}
		var bootstrapResp struct {
			BootstrapToken string `json:"bootstrap_token"`
		}
		decodeJSONBody(t, bootstrapW, &bootstrapResp)

		pairW := callJSONHandler(t, s.handleAuthPairComplete, http.MethodPost, "/v1/auth/pair/complete", "", fmt.Sprintf(`{"token":%q,"container_id":%q}`, bootstrapResp.BootstrapToken, containerID))
		if pairW.Code != http.StatusOK {
			t.Fatalf("pair complete failed for %s: code=%d body=%s", agentID, pairW.Code, pairW.Body.String())
		}
		var pairResp struct {
			GrantID string `json:"grant_id"`
		}
		decodeJSONBody(t, pairW, &pairResp)

		mintW := callJSONHandler(t, s.handleAuthSessionMint, http.MethodPost, "/v1/auth/session/mint", "", fmt.Sprintf(`{"grant_id":%q}`, pairResp.GrantID))
		if mintW.Code != http.StatusOK {
			t.Fatalf("session mint failed for %s: code=%d body=%s", agentID, mintW.Code, mintW.Body.String())
		}
		var mintResp struct {
			SessionToken string `json:"session_token"`
		}
		decodeJSONBody(t, mintW, &mintResp)
		return mintResp.SessionToken
	}

	invalidSessionW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", "invalid-session", `{"agent_id":"agent-1","task_id":"invalid-session","reason":"invalid","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-invalid","workdir_fingerprint":"wd-invalid"}`)
	if invalidSessionW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid session, got %d body=%s", invalidSessionW.Code, invalidSessionW.Body.String())
	}

	traversalW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", mintResp.SessionToken, `{"agent_id":"agent-1","task_id":"traversal","reason":"traversal","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-traversal","workdir_fingerprint":"wd-traversal","env_path":"../outside.env"}`)
	if traversalW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for env-path traversal, got %d body=%s", traversalW.Code, traversalW.Body.String())
	}
	if !strings.Contains(traversalW.Body.String(), "outside allowed root") {
		t.Fatalf("expected traversal rejection detail, got %q", traversalW.Body.String())
	}

	envRequestW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", mintResp.SessionToken, `{"agent_id":"agent-1","task_id":"env-exec","reason":"env execute","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-env","workdir_fingerprint":"wd-env","env_path":".env"}`)
	if envRequestW.Code != http.StatusOK {
		t.Fatalf("env request failed: code=%d body=%s", envRequestW.Code, envRequestW.Body.String())
	}
	var envRequestResp struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
	}
	decodeJSONBody(t, envRequestW, &envRequestResp)
	if envRequestResp.Status != string(domain.RequestPending) {
		t.Fatalf("env request status = %q, want %q", envRequestResp.Status, domain.RequestPending)
	}

	envApproveW := callJSONHandler(t, s.handleApprove, http.MethodPost, "/v1/leases/approve?request_id="+envRequestResp.RequestID, "operator-token", `{"ttl_minutes":5}`)
	if envApproveW.Code != http.StatusOK {
		t.Fatalf("env approve failed: code=%d body=%s", envApproveW.Code, envApproveW.Body.String())
	}
	var envApproveResp struct {
		LeaseToken string `json:"lease_token"`
		Status     string `json:"status"`
	}
	decodeJSONBody(t, envApproveW, &envApproveResp)
	if envApproveResp.Status != "approved" {
		t.Fatalf("env approve status = %q, want approved", envApproveResp.Status)
	}

	execW := callJSONHandler(t, s.handleExecute, http.MethodPost, "/v1/leases/execute", mintResp.SessionToken, fmt.Sprintf(`{"lease_token":%q,"command":["bash","-lc","echo -n $GITHUB_TOKEN"],"secrets":["github_token"],"command_fingerprint":"fp-env","workdir_fingerprint":"wd-env"}`, envApproveResp.LeaseToken))
	if execW.Code != http.StatusOK {
		t.Fatalf("execute failed: code=%d body=%s", execW.Code, execW.Body.String())
	}
	var execResp struct {
		ExitCode     int    `json:"exit_code"`
		StdoutStderr string `json:"stdout_stderr"`
	}
	decodeJSONBody(t, execW, &execResp)
	if execResp.ExitCode != 0 {
		t.Fatalf("execute exit_code = %d, want 0", execResp.ExitCode)
	}
	if execResp.StdoutStderr != "from_dotenv" {
		t.Fatalf("execute output = %q, want from_dotenv", execResp.StdoutStderr)
	}

	plaintextAccessW := callJSONHandler(t, s.handleAccess, http.MethodPost, "/v1/leases/access", mintResp.SessionToken, fmt.Sprintf(`{"lease_token":%q,"secret":"github_token","command_fingerprint":"fp-env","workdir_fingerprint":"wd-env"}`, envApproveResp.LeaseToken))
	if plaintextAccessW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for plaintext access policy, got %d body=%s", plaintextAccessW.Code, plaintextAccessW.Body.String())
	}

	denyRequestW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", mintResp.SessionToken, `{"agent_id":"agent-1","task_id":"env-deny","reason":"deny path","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-deny","workdir_fingerprint":"wd-deny","env_path":".env"}`)
	if denyRequestW.Code != http.StatusOK {
		t.Fatalf("deny request creation failed: code=%d body=%s", denyRequestW.Code, denyRequestW.Body.String())
	}
	var denyRequestResp struct {
		RequestID string `json:"request_id"`
	}
	decodeJSONBody(t, denyRequestW, &denyRequestResp)

	denyW := callJSONHandler(t, s.handleDeny, http.MethodPost, "/v1/leases/deny?request_id="+denyRequestResp.RequestID, "operator-token", `{"reason":"operator_rejected"}`)
	if denyW.Code != http.StatusOK {
		t.Fatalf("deny failed: code=%d body=%s", denyW.Code, denyW.Body.String())
	}

	denyStatusW := callJSONHandler(t, s.handleRequestStatus, http.MethodGet, "/v1/requests/status?request_id="+denyRequestResp.RequestID, mintResp.SessionToken, "")
	if denyStatusW.Code != http.StatusOK {
		t.Fatalf("request status failed: code=%d body=%s", denyStatusW.Code, denyStatusW.Body.String())
	}
	var denyStatusResp struct {
		Status string `json:"status"`
	}
	decodeJSONBody(t, denyStatusW, &denyStatusResp)
	if denyStatusResp.Status != string(domain.RequestDenied) {
		t.Fatalf("deny status = %q, want %q", denyStatusResp.Status, domain.RequestDenied)
	}

	denyByRequestW := callJSONHandler(t, s.handleLeaseByRequest, http.MethodGet, "/v1/leases/by-request?request_id="+denyRequestResp.RequestID, mintResp.SessionToken, "")
	if denyByRequestW.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for denied request lease lookup, got %d body=%s", denyByRequestW.Code, denyByRequestW.Body.String())
	}

	reuseSession := mintSession("agent-reuse", "container-reuse")
	reuseCreateW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", reuseSession, `{"agent_id":"agent-reuse","task_id":"reuse-1","reason":"first","ttl_minutes":5,"secrets":["npm_token"],"command_fingerprint":"fp-reuse","workdir_fingerprint":"wd-reuse"}`)
	if reuseCreateW.Code != http.StatusOK {
		t.Fatalf("reuse create failed: code=%d body=%s", reuseCreateW.Code, reuseCreateW.Body.String())
	}
	var reuseCreateResp struct {
		RequestID string `json:"request_id"`
	}
	decodeJSONBody(t, reuseCreateW, &reuseCreateResp)

	reuseApproveW := callJSONHandler(t, s.handleApprove, http.MethodPost, "/v1/leases/approve?request_id="+reuseCreateResp.RequestID, "operator-token", `{"ttl_minutes":5}`)
	if reuseApproveW.Code != http.StatusOK {
		t.Fatalf("reuse approve failed: code=%d body=%s", reuseApproveW.Code, reuseApproveW.Body.String())
	}
	var reuseApproveResp struct {
		LeaseToken string `json:"lease_token"`
	}
	decodeJSONBody(t, reuseApproveW, &reuseApproveResp)

	reuseW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", reuseSession, `{"agent_id":"agent-reuse","task_id":"reuse-2","reason":"equivalent","ttl_minutes":5,"secrets":[" npm_token "],"command_fingerprint":"fp-reuse","workdir_fingerprint":"wd-reuse"}`)
	if reuseW.Code != http.StatusOK {
		t.Fatalf("reuse request failed: code=%d body=%s", reuseW.Code, reuseW.Body.String())
	}
	var reuseResp struct {
		Status     string `json:"status"`
		RequestID  string `json:"request_id"`
		LeaseToken string `json:"lease_token"`
	}
	decodeJSONBody(t, reuseW, &reuseResp)
	if reuseResp.Status != "reused" {
		t.Fatalf("reuse status = %q, want reused", reuseResp.Status)
	}
	if reuseResp.RequestID != reuseCreateResp.RequestID {
		t.Fatalf("reuse request_id = %q, want %q", reuseResp.RequestID, reuseCreateResp.RequestID)
	}
	if reuseResp.LeaseToken != reuseApproveResp.LeaseToken {
		t.Fatalf("reuse lease_token = %q, want %q", reuseResp.LeaseToken, reuseApproveResp.LeaseToken)
	}

	cooldownSession := mintSession("agent-cool", "container-cool")
	cooldownCreateW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", cooldownSession, `{"agent_id":"agent-cool","task_id":"cool-1","reason":"first","ttl_minutes":5,"secrets":["github_token","npm_token"],"command_fingerprint":"fp-cool","workdir_fingerprint":"wd-cool"}`)
	if cooldownCreateW.Code != http.StatusOK {
		t.Fatalf("cooldown create failed: code=%d body=%s", cooldownCreateW.Code, cooldownCreateW.Body.String())
	}
	cooldownW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", cooldownSession, `{"agent_id":"agent-cool","task_id":"cool-2","reason":"repeat","ttl_minutes":5,"secrets":[" npm_token ","github_token"],"command_fingerprint":"fp-cool","workdir_fingerprint":"wd-cool"}`)
	if cooldownW.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for cooldown, got %d body=%s", cooldownW.Code, cooldownW.Body.String())
	}
	if got := cooldownW.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("cooldown Retry-After = %q, want 60", got)
	}
	if !strings.Contains(cooldownW.Body.String(), "equivalent request cooldown active") {
		t.Fatalf("expected cooldown guidance, got %q", cooldownW.Body.String())
	}

	pendingCapSession := mintSession("agent-cap", "container-cap")
	pendingCapFirstW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", pendingCapSession, `{"agent_id":"agent-cap","task_id":"cap-1","reason":"first","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-cap-1","workdir_fingerprint":"wd-cap-1"}`)
	if pendingCapFirstW.Code != http.StatusOK {
		t.Fatalf("pending-cap first request failed: code=%d body=%s", pendingCapFirstW.Code, pendingCapFirstW.Body.String())
	}
	pendingCapSecondW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", pendingCapSession, `{"agent_id":"agent-cap","task_id":"cap-2","reason":"second","ttl_minutes":5,"secrets":["npm_token"],"command_fingerprint":"fp-cap-2","workdir_fingerprint":"wd-cap-2"}`)
	if pendingCapSecondW.Code != http.StatusOK {
		t.Fatalf("pending-cap second request failed: code=%d body=%s", pendingCapSecondW.Code, pendingCapSecondW.Body.String())
	}
	pendingCapW := callJSONHandler(t, s.handleRequest, http.MethodPost, "/v1/leases/request", pendingCapSession, `{"agent_id":"agent-cap","task_id":"cap-3","reason":"third","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-cap-3","workdir_fingerprint":"wd-cap-3"}`)
	if pendingCapW.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for pending cap, got %d body=%s", pendingCapW.Code, pendingCapW.Body.String())
	}
	if got := pendingCapW.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("pending-cap Retry-After = %q, want 60", got)
	}
	if !strings.Contains(pendingCapW.Body.String(), "pending request cap reached") {
		t.Fatalf("expected pending-cap guidance, got %q", pendingCapW.Body.String())
	}

	requiredEvents := []string{
		"auth_bootstrap_created",
		"auth_pair_completed",
		"auth_session_minted",
		app.AuditEventEnvPathConfirmed,
		app.AuditEventEnvPathRejected,
		"operator_denied_request",
		app.AuditEventRequestReusedActiveLease,
		app.AuditEventRequestThrottledCooldown,
		app.AuditEventRequestThrottledPendingCap,
		"plaintext_secret_access_blocked",
	}
	for _, name := range requiredEvents {
		if !hasAuditEventNamed(audit, name) {
			t.Fatalf("expected audit event %q to be present", name)
		}
	}
}

func callJSONHandler(t *testing.T, handler func(http.ResponseWriter, *http.Request), method, path, bearer, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
	if strings.TrimSpace(bearer) != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func decodeJSONBody(t *testing.T, recorder *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), out); err != nil {
		t.Fatalf("decode json response: %v; body=%s", err, recorder.Body.String())
	}
}

func hasAuditEventNamed(audit *captureAudit, name string) bool {
	if audit == nil {
		return false
	}
	for _, event := range audit.events {
		if event.Event == name {
			return true
		}
	}
	return false
}
