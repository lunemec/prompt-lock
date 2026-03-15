package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type fakeCommandRunner struct {
	requests []CommandRunRequest
	result   CommandRunResult
	err      error
}

func (r *fakeCommandRunner) Run(_ context.Context, req CommandRunRequest) (CommandRunResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return CommandRunResult{}, r.err
	}
	return r.result, nil
}

type fakeControlPlanePolicy struct {
	executeRequestErr error
	executeCommandErr error
	resolveExecuteErr error
	networkErr        error
	hostDockerErr     error
	resolveHostErr    error
	resolvedExecute   ResolvedCommand
	resolvedHost      ResolvedCommand
	outputMode        string
	clampedTimeout    int
}

func (p fakeControlPlanePolicy) ValidateExecuteRequest(string, ExecuteRequest) error {
	return p.executeRequestErr
}

func (p fakeControlPlanePolicy) ValidateExecuteCommand([]string) error {
	return p.executeCommandErr
}

func (p fakeControlPlanePolicy) ResolveExecuteCommand([]string) (ResolvedCommand, error) {
	if p.resolveExecuteErr != nil {
		return ResolvedCommand{}, p.resolveExecuteErr
	}
	return p.resolvedExecute, nil
}

func (p fakeControlPlanePolicy) ValidateNetworkEgress([]string, string) error {
	return p.networkErr
}

func (p fakeControlPlanePolicy) ValidateHostDockerCommand([]string) error {
	return p.hostDockerErr
}

func (p fakeControlPlanePolicy) ResolveHostDockerCommand([]string) (ResolvedCommand, error) {
	if p.resolveHostErr != nil {
		return ResolvedCommand{}, p.resolveHostErr
	}
	return p.resolvedHost, nil
}

func (p fakeControlPlanePolicy) ApplyOutputSecurity(in string) string {
	if p.outputMode == "none" {
		return ""
	}
	if p.outputMode == "raw" {
		return in
	}
	return redactOutput(in)
}

func (p fakeControlPlanePolicy) ClampTimeout(int) int {
	if p.clampedTimeout > 0 {
		return p.clampedTimeout
	}
	return 30
}

func TestExecuteWithLeaseUseCaseRejectsIntentWidening(t *testing.T) {
	now := time.Date(2026, 3, 15, 18, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "req-1", AgentID: "agent-1", TaskID: "task-1", Intent: "run_tests", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "lease-1", RequestID: "req-1", AgentID: "agent-1", TaskID: "task-1", Intent: "run_tests", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	runner := &fakeCommandRunner{}
	usecase := ExecuteWithLeaseUseCase{
		Service: &Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        &auditBuf{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "unused" },
			NewLeaseTok:  func() string { return "unused" },
		},
		Policy: fakeControlPlanePolicy{
			resolvedExecute: ResolvedCommand{Path: "/usr/bin/curl", Args: []string{"https://api.github.com"}, SearchPath: "/usr/bin"},
			outputMode:      "raw",
			clampedTimeout:  15,
		},
		SecurityProfile: "hardened",
		Runner:          runner,
		MaxOutputBytes:  4096,
		AmbientEnv:      []string{"PATH=/usr/bin"},
	}

	_, err := usecase.Execute(context.Background(), ExecuteWithLeaseInput{
		ActorType:           "agent",
		ActorID:             "agent-1",
		LeaseToken:          "lease-1",
		Intent:              "deploy",
		Command:             []string{"curl", "https://api.github.com"},
		Secrets:             []string{"github_token"},
		CommandFingerprint:  "fp",
		WorkdirFingerprint:  "wd",
		RequestedTimeoutSec: 15,
	})
	if err == nil {
		t.Fatalf("expected execute-time intent widening to fail")
	}
	if len(runner.requests) != 0 {
		t.Fatalf("expected runner to stay blocked, got %#v", runner.requests)
	}
}

func TestExecuteWithLeaseUseCaseDoesNotReadSecretBackendBeforeCommandResolution(t *testing.T) {
	now := time.Date(2026, 3, 15, 18, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	secrets := &countingSecretStore{value: "ok123"}
	audit := &auditBuf{}
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		Status:             domain.RequestApproved,
		CreatedAt:          now,
	})
	_ = store.SaveLease(domain.Lease{
		Token:              "lease-1",
		RequestID:          "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		ExpiresAt:          now.Add(5 * time.Minute),
	})

	runner := &fakeCommandRunner{}
	usecase := ExecuteWithLeaseUseCase{
		Service: &Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      secrets,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "unused" },
			NewLeaseTok:  func() string { return "unused" },
		},
		Policy: fakeControlPlanePolicy{
			resolveExecuteErr: errors.New("command resolution failed"),
			outputMode:        "raw",
			clampedTimeout:    15,
		},
		SecurityProfile: "hardened",
		Runner:          runner,
		MaxOutputBytes:  4096,
		AmbientEnv:      []string{"PATH=/usr/bin"},
	}

	_, err := usecase.Execute(context.Background(), ExecuteWithLeaseInput{
		ActorType:           "agent",
		ActorID:             "agent-1",
		LeaseToken:          "lease-1",
		Intent:              "run_tests",
		Command:             []string{"go", "version"},
		Secrets:             []string{"github_token"},
		CommandFingerprint:  "fp",
		WorkdirFingerprint:  "wd",
		RequestedTimeoutSec: 15,
	})
	if err == nil || !strings.Contains(err.Error(), "command resolution failed") {
		t.Fatalf("expected command resolution failure, got %v", err)
	}
	if secrets.calls != 0 {
		t.Fatalf("expected secret backend read to stay blocked, got %d calls", secrets.calls)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("expected runner to stay blocked, got %#v", runner.requests)
	}
	assertAuditOmitsSecretAccessEvents(t, audit.events)
}

func TestExecuteWithLeaseUseCaseDoesNotReadEnvPathSecretsBeforeCommandResolution(t *testing.T) {
	now := time.Date(2026, 3, 15, 18, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	envStore := &countingEnvPathSecretStore{
		resolved:  map[string]string{"github_token": "dotenv-value"},
		canonical: "/workspace/.env",
	}
	audit := &auditBuf{}
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestApproved,
		CreatedAt:          now,
	})
	_ = store.SaveLease(domain.Lease{
		Token:              "lease-1",
		RequestID:          "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		ExpiresAt:          now.Add(5 * time.Minute),
	})

	runner := &fakeCommandRunner{}
	usecase := ExecuteWithLeaseUseCase{
		Service: &Service{
			Policy:         domain.DefaultPolicy(),
			Requests:       store,
			Leases:         store,
			Secrets:        &countingSecretStore{value: "primary"},
			EnvPathSecrets: envStore,
			Audit:          audit,
			Now:            func() time.Time { return now },
			NewRequestID:   func() string { return "unused" },
			NewLeaseTok:    func() string { return "unused" },
		},
		Policy: fakeControlPlanePolicy{
			resolveExecuteErr: errors.New("command resolution failed"),
			outputMode:        "raw",
			clampedTimeout:    15,
		},
		SecurityProfile: "hardened",
		Runner:          runner,
		MaxOutputBytes:  4096,
		AmbientEnv:      []string{"PATH=/usr/bin"},
	}

	_, err := usecase.Execute(context.Background(), ExecuteWithLeaseInput{
		ActorType:           "agent",
		ActorID:             "agent-1",
		LeaseToken:          "lease-1",
		Intent:              "run_tests",
		Command:             []string{"go", "version"},
		Secrets:             []string{"github_token"},
		CommandFingerprint:  "fp",
		WorkdirFingerprint:  "wd",
		RequestedTimeoutSec: 15,
	})
	if err == nil || !strings.Contains(err.Error(), "command resolution failed") {
		t.Fatalf("expected command resolution failure, got %v", err)
	}
	if envStore.resolveCalls != 0 {
		t.Fatalf("expected env-path read to stay blocked, got %d calls", envStore.resolveCalls)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("expected runner to stay blocked, got %#v", runner.requests)
	}
	assertAuditOmitsSecretAccessEvents(t, audit.events)
}

func assertAuditOmitsSecretAccessEvents(t *testing.T, events []ports.AuditEvent) {
	t.Helper()
	for _, event := range events {
		if event.Event == AuditEventSecretAccessStarted || event.Event == "secret_access" {
			t.Fatalf("did not expect secret access audit event before command resolution, got %+v", events)
		}
	}
}

func TestHostDockerExecuteUseCaseReturnsAuditWarningAfterCompletionAuditFailure(t *testing.T) {
	now := time.Date(2026, 3, 15, 18, 0, 0, 0, time.UTC)
	audit := &scriptedAuditBuf{failAt: 2}
	runner := &fakeCommandRunner{
		result: CommandRunResult{Output: "docker ps output", ExitCode: 0},
	}
	usecase := HostDockerExecuteUseCase{
		Service: &Service{
			Audit: audit,
			Now:   func() time.Time { return now },
			AuditFailureHandler: func(err error) error {
				return errors.New("durability persistence unavailable; broker closed for safety")
			},
		},
		Policy: fakeControlPlanePolicy{
			resolvedHost:   ResolvedCommand{Path: "/usr/bin/docker", Args: []string{"ps"}, SearchPath: "/usr/bin"},
			clampedTimeout: 30,
		},
		Runner:         runner,
		AmbientEnv:     []string{"PATH=/usr/bin"},
		MaxOutputBytes: 4096,
		TimeoutSec:     30,
	}

	result, err := usecase.Execute(context.Background(), HostDockerExecuteInput{
		ActorType: "operator",
		ActorID:   "operator-1",
		Command:   []string{"docker", "ps"},
	})
	if err != nil {
		t.Fatalf("host docker execute: %v", err)
	}
	if result.AuditWarning == "" {
		t.Fatalf("expected audit warning after completion audit failure")
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("expected one runner request, got %#v", runner.requests)
	}
}

func TestExecuteWithLeaseUseCaseDoesNotInheritAmbientEnvWithoutInjection(t *testing.T) {
	now := time.Date(2026, 3, 15, 18, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "req-1", AgentID: "agent-1", TaskID: "task-1", Intent: "run_tests", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "lease-1", RequestID: "req-1", AgentID: "agent-1", TaskID: "task-1", Intent: "run_tests", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	runner := &fakeCommandRunner{
		result: CommandRunResult{Output: "ok", ExitCode: 0},
	}
	usecase := ExecuteWithLeaseUseCase{
		Service: &Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        &auditBuf{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "unused" },
			NewLeaseTok:  func() string { return "unused" },
		},
		Policy: fakeControlPlanePolicy{
			resolvedExecute: ResolvedCommand{Path: "/usr/bin/curl", Args: []string{"https://api.github.com"}, SearchPath: "/trusted/bin:/usr/bin"},
			outputMode:      "raw",
			clampedTimeout:  15,
		},
		SecurityProfile: "hardened",
		Runner:          runner,
		MaxOutputBytes:  4096,
	}

	t.Setenv("HOME", "/unexpected-home")
	_, err := usecase.Execute(context.Background(), ExecuteWithLeaseInput{
		ActorType:           "agent",
		ActorID:             "agent-1",
		LeaseToken:          "lease-1",
		Intent:              "run_tests",
		Command:             []string{"curl", "https://api.github.com"},
		Secrets:             []string{"github_token"},
		CommandFingerprint:  "fp",
		WorkdirFingerprint:  "wd",
		RequestedTimeoutSec: 15,
	})
	if err != nil {
		t.Fatalf("execute with lease: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("expected one runner request, got %#v", runner.requests)
	}
	for _, entry := range runner.requests[0].Env {
		if strings.Contains(entry, "HOME=/unexpected-home") {
			t.Fatalf("expected nil AmbientEnv to avoid inheriting host env, got %#v", runner.requests[0].Env)
		}
	}
}

func TestHostDockerExecuteUseCaseDoesNotInheritAmbientEnvWithoutInjection(t *testing.T) {
	now := time.Date(2026, 3, 15, 18, 0, 0, 0, time.UTC)
	runner := &fakeCommandRunner{
		result: CommandRunResult{Output: "docker ps output", ExitCode: 0},
	}
	usecase := HostDockerExecuteUseCase{
		Service: &Service{
			Audit: &auditBuf{},
			Now:   func() time.Time { return now },
		},
		Policy: fakeControlPlanePolicy{
			resolvedHost:   ResolvedCommand{Path: "/usr/bin/docker", Args: []string{"ps"}, SearchPath: "/trusted/bin:/usr/bin"},
			clampedTimeout: 30,
		},
		Runner:         runner,
		MaxOutputBytes: 4096,
		TimeoutSec:     30,
	}

	t.Setenv("HOME", "/unexpected-home")
	_, err := usecase.Execute(context.Background(), HostDockerExecuteInput{
		ActorType: "operator",
		ActorID:   "operator-1",
		Command:   []string{"docker", "ps"},
	})
	if err != nil {
		t.Fatalf("host docker execute: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("expected one runner request, got %#v", runner.requests)
	}
	for _, entry := range runner.requests[0].Env {
		if strings.Contains(entry, "HOME=/unexpected-home") {
			t.Fatalf("expected nil AmbientEnv to avoid inheriting host env, got %#v", runner.requests[0].Env)
		}
	}
}
