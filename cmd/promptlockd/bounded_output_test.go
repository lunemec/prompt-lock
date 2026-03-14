package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestRunCommandWithBoundedOutputTruncates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is unix-specific")
	}

	cmd := exec.Command("sh", "-c", "i=0; while [ $i -lt 128 ]; do printf x; i=$((i+1)); done")
	out, exitCode, err := runCommandWithBoundedOutput(cmd, 16)
	if err != nil {
		t.Fatalf("runCommandWithBoundedOutput returned error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if out != strings.Repeat("x", 16) {
		t.Fatalf("output = %q, want %q", out, strings.Repeat("x", 16))
	}
}

func TestHandleExecuteCapsOutputAtMaxBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is unix-specific")
	}

	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := &server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy:  config.ExecutionPolicy{ExactMatchExecutables: []string{"sh"}, OutputSecurityMode: "raw", MaxOutputBytes: 16, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		authStore:   aStore,
		now:         func() time.Time { return now },
	}

	payload := `{"lease_token":"l1","command":["sh","-c","i=0; while [ $i -lt 128 ]; do printf x; i=$((i+1)); done"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var out struct {
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.StdoutStderr != strings.Repeat("x", 16) {
		t.Fatalf("stdout_stderr = %q, want %q", out.StdoutStderr, strings.Repeat("x", 16))
	}
}

type hostDockerBoundedOutputPolicy struct{}

func (hostDockerBoundedOutputPolicy) ValidateExecuteRequest(string, app.ExecuteRequest) error {
	return nil
}

func (hostDockerBoundedOutputPolicy) ValidateExecuteCommand([]string) error { return nil }

func (hostDockerBoundedOutputPolicy) ResolveExecuteCommand([]string) (app.ResolvedCommand, error) {
	return app.ResolvedCommand{}, nil
}

func (hostDockerBoundedOutputPolicy) ValidateNetworkEgress([]string, string) error { return nil }
func (hostDockerBoundedOutputPolicy) ValidateHostDockerCommand([]string) error     { return nil }

func (hostDockerBoundedOutputPolicy) ResolveHostDockerCommand([]string) (app.ResolvedCommand, error) {
	return app.ResolvedCommand{
		Path: "sh",
		Args: []string{"-c", "i=0; while [ $i -lt 128 ]; do printf x; i=$((i+1)); done"},
	}, nil
}

func (hostDockerBoundedOutputPolicy) ApplyOutputSecurity(in string) string { return in }
func (hostDockerBoundedOutputPolicy) ClampTimeout(requested int) int       { return requested }

func TestHandleHostDockerExecuteCapsOutputAtMaxBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is unix-specific")
	}

	s := &server{
		svc:           app.Service{Audit: testAudit{}},
		execPolicy:    config.ExecutionPolicy{MaxOutputBytes: 16},
		hostOpsPolicy: config.HostOpsPolicy{DockerTimeoutSec: 30},
		policyEngine:  hostDockerBoundedOutputPolicy{},
		now:           time.Now,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/host/docker/execute", bytes.NewBufferString(`{"command":["docker","ps"]}`))
	w := httptest.NewRecorder()
	s.handleHostDockerExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var out struct {
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.StdoutStderr != strings.Repeat("x", 16) {
		t.Fatalf("stdout_stderr = %q, want %q", out.StdoutStderr, strings.Repeat("x", 16))
	}
}
