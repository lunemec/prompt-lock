package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type scriptedAudit struct {
	mu     sync.Mutex
	calls  int
	failAt map[int]error
	events []ports.AuditEvent
}

func (a *scriptedAudit) Write(ev ports.AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	a.events = append(a.events, ev)
	if err, ok := a.failAt[a.calls]; ok {
		return err
	}
	return nil
}

func TestHandleHostDockerExecuteRejectsClosedDurabilityGateBeforeRunningCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script helper is unix-specific")
	}

	marker, helper := hostDockerMarkerHelper(t)
	s := &server{
		svc:              app.Service{Audit: testAudit{}},
		execPolicy:       config.ExecutionPolicy{MaxOutputBytes: 64},
		hostOpsPolicy:    config.HostOpsPolicy{DockerTimeoutSec: 30},
		policyEngine:     hostDockerEnvPolicy{helperPath: helper},
		durabilityClosed: true,
		now:              time.Now,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/host/docker/execute", bytes.NewBufferString(`{"command":["docker","ps"]}`))
	w := httptest.NewRecorder()
	s.handleHostDockerExecute(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with closed durability gate, got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected helper command to stay blocked, stat err=%v", err)
	}
}

func TestHandleHostDockerExecuteFailsBeforeRunningCommandWhenStartAuditFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script helper is unix-specific")
	}

	marker, helper := hostDockerMarkerHelper(t)
	audit := &scriptedAudit{failAt: map[int]error{1: errors.New("audit disk offline")}}
	s := &server{
		svc:           app.Service{Audit: audit},
		execPolicy:    config.ExecutionPolicy{MaxOutputBytes: 64},
		hostOpsPolicy: config.HostOpsPolicy{DockerTimeoutSec: 30},
		policyEngine:  hostDockerEnvPolicy{helperPath: helper},
		now:           time.Now,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/host/docker/execute", bytes.NewBufferString(`{"command":["docker","ps"]}`))
	w := httptest.NewRecorder()
	s.handleHostDockerExecute(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when start audit fails, got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected helper command to stay blocked, stat err=%v", err)
	}
}

func TestHandleHostDockerExecuteReturnsAuditWarningWhenCompletionAuditFailsAfterCommandRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script helper is unix-specific")
	}

	marker, helper := hostDockerMarkerHelper(t)
	audit := &scriptedAudit{failAt: map[int]error{2: errors.New("audit disk offline")}}
	s := &server{
		svc:           app.Service{Audit: audit},
		execPolicy:    config.ExecutionPolicy{MaxOutputBytes: 64},
		hostOpsPolicy: config.HostOpsPolicy{DockerTimeoutSec: 30},
		policyEngine:  hostDockerEnvPolicy{helperPath: helper},
		now:           time.Now,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/host/docker/execute", bytes.NewBufferString(`{"command":["docker","ps"]}`))
	w := httptest.NewRecorder()
	s.handleHostDockerExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after command runs even if completion audit fails, got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected helper command to run, stat err=%v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["audit_warning"] == "" {
		t.Fatalf("expected audit_warning in response, got %+v", out)
	}
	if !s.durabilityClosed {
		t.Fatalf("expected completion audit failure to close durability gate")
	}
}

func TestHandleExecuteFailsBeforeRunningCommandWhenStartAuditFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script helper is unix-specific")
	}

	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	markerDir := t.TempDir()
	markerPath := filepath.Join(markerDir, "execute-ran")
	helperPath := filepath.Join(markerDir, "marker")
	writeExecutableScript(t, helperPath, "printf ran > "+shellQuote(markerPath)+"\nprintf command-ok")

	audit := &scriptedAudit{failAt: map[int]error{2: errors.New("audit disk offline")}}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "r1" },
			NewLeaseTok:  func() string { return "l1" },
		},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"marker"},
			CommandSearchPaths:    []string{markerDir},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        64,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(`{"lease_token":"l1","command":["marker"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when start audit fails, got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected helper command to stay blocked, stat err=%v", err)
	}
}

func TestHandleExecuteReturnsAuditWarningWhenCompletionAuditFailsAfterCommandRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script helper is unix-specific")
	}

	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	markerDir := t.TempDir()
	markerPath := filepath.Join(markerDir, "execute-ran")
	helperPath := filepath.Join(markerDir, "marker")
	writeExecutableScript(t, helperPath, "printf ran > "+shellQuote(markerPath)+"\nprintf command-ok")

	audit := &scriptedAudit{failAt: map[int]error{3: errors.New("audit disk offline")}}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "r1" },
			NewLeaseTok:  func() string { return "l1" },
		},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"marker"},
			CommandSearchPaths:    []string{markerDir},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        64,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(`{"lease_token":"l1","command":["marker"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after command runs even if completion audit fails, got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected helper command to run, stat err=%v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["stdout_stderr"] != "command-ok" {
		t.Fatalf("expected command output, got %+v", out)
	}
	if out["audit_warning"] == "" {
		t.Fatalf("expected audit_warning in response, got %+v", out)
	}
	if !s.durabilityClosed {
		t.Fatalf("expected completion audit failure to close durability gate")
	}
}

func hostDockerMarkerHelper(t *testing.T) (string, string) {
	t.Helper()
	td := t.TempDir()
	markerPath := filepath.Join(td, "host-docker-ran")
	helperPath := filepath.Join(td, "host-docker-helper.sh")
	writeExecutableScript(t, helperPath, "printf ran > "+shellQuote(markerPath)+"\nprintf host-docker-ok")
	return markerPath, helperPath
}

func shellQuote(path string) string {
	return "'" + bytes.NewBufferString(path).String() + "'"
}
