package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/envpath"
	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type captureAudit struct {
	events []ports.AuditEvent
}

func (c *captureAudit) Write(e ports.AuditEvent) error {
	c.events = append(c.events, e)
	return nil
}

func TestExecuteUsesApprovedEnvPathSecrets(t *testing.T) {
	now := time.Now().UTC()
	root := t.TempDir()
	envDir := filepath.Join(root, "secrets")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir env dir: %v", err)
	}
	envFile := filepath.Join(envDir, ".env")
	if err := os.WriteFile(envFile, []byte("github_token=from_dotenv\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	expectedCanonicalPath, err := filepath.EvalSymlinks(envFile)
	if err != nil {
		t.Fatalf("canonicalize env file: %v", err)
	}

	store := memory.NewStore()
	store.SetSecret("github_token", "from_primary_secret_store")
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-env-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Reason:             "env path request",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-env",
		WorkdirFingerprint: "wd-env",
		EnvPath:            "secrets/.env",
		EnvPathCanonical:   expectedCanonicalPath,
		Status:             domain.RequestApproved,
		CreatedAt:          now,
	})
	_ = store.SaveLease(domain.Lease{
		Token:              "lease-env-1",
		RequestID:          "req-env-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-env",
		WorkdirFingerprint: "wd-env",
		ExpiresAt:          now.Add(5 * time.Minute),
	})

	envStore, err := envpath.New(root)
	if err != nil {
		t.Fatalf("new env path store: %v", err)
	}

	s := &server{
		svc: app.Service{
			Policy:         domain.DefaultPolicy(),
			Requests:       store,
			Leases:         store,
			Secrets:        store,
			EnvPathSecrets: envStore,
			Audit:          &captureAudit{},
			Now:            func() time.Time { return now },
			NewRequestID:   func() string { return "unused-request-id" },
			NewLeaseTok:    func() string { return "unused-lease-token" },
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{EnableAuth: false, AllowPlaintextSecretReturn: false},
		execPolicy:  config.ExecutionPolicy{ExactMatchExecutables: []string{"bash"}, DenylistSubstrings: []string{"printenv"}, MaxOutputBytes: 65536, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		now:         func() time.Time { return now },
	}

	payload := `{"lease_token":"lease-env-1","command":["bash","-lc","echo -n $GITHUB_TOKEN"],"secrets":["github_token"],"command_fingerprint":"fp-env","workdir_fingerprint":"wd-env"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var out struct {
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode execute response: %v", err)
	}
	if out.StdoutStderr != "from_dotenv" {
		t.Fatalf("expected command output from .env-backed secret, got %q", out.StdoutStderr)
	}
}

func TestEnvPathDecisionsAuditedOnApproveAndDeny(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	audit := &captureAudit{}

	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-env-approve",
		AgentID:            "agent-1",
		TaskID:             "task-approve",
		Reason:             "approve env request",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-approve",
		WorkdirFingerprint: "wd-approve",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/project/.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	})
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-env-deny",
		AgentID:            "agent-2",
		TaskID:             "task-deny",
		Reason:             "deny env request",
		TTLMinutes:         5,
		Secrets:            []string{"npm_token"},
		CommandFingerprint: "fp-deny",
		WorkdirFingerprint: "wd-deny",
		EnvPath:            "../.env",
		EnvPathCanonical:   "/workspace/project/../.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	})

	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "unused-request-id" },
			NewLeaseTok:  func() string { return "lease-approved" },
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{EnableAuth: false},
		now:         func() time.Time { return now },
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-env-approve", bytes.NewBufferString(`{"ttl_minutes":5}`))
	approveW := httptest.NewRecorder()
	s.handleApprove(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("approve failed: code=%d body=%s", approveW.Code, approveW.Body.String())
	}

	denyReq := httptest.NewRequest(http.MethodPost, "/v1/leases/deny?request_id=req-env-deny", bytes.NewBufferString(`{"reason":"operator_rejected"}`))
	denyW := httptest.NewRecorder()
	s.handleDeny(denyW, denyReq)
	if denyW.Code != http.StatusOK {
		t.Fatalf("deny failed: code=%d body=%s", denyW.Code, denyW.Body.String())
	}

	var confirmed, rejected bool
	for _, ev := range audit.events {
		if ev.Event == app.AuditEventEnvPathConfirmed && ev.RequestID == "req-env-approve" {
			confirmed = true
		}
		if ev.Event == app.AuditEventEnvPathRejected && ev.RequestID == "req-env-deny" {
			rejected = true
		}
	}
	if !confirmed {
		t.Fatalf("expected %s event for approved env-path request", app.AuditEventEnvPathConfirmed)
	}
	if !rejected {
		t.Fatalf("expected %s event for denied env-path request", app.AuditEventEnvPathRejected)
	}
}
