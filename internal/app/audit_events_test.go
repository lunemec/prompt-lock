package app

import (
	"errors"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func TestRequestLeaseWithPolicyAuditsActiveLeaseReuse(t *testing.T) {
	store := memory.NewStore()
	audit := &auditBuf{}
	now := time.Date(2026, 3, 11, 18, 0, 0, 0, time.UTC)
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_active",
		RequestID:          "req_existing",
		AgentID:            "agent1",
		TaskID:             "task-old",
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(10 * time.Minute),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        audit,
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	input := RequestPolicyInput{
		AgentID:            "agent1",
		Secrets:            []string{"npm_token", "github_token", "github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
	}
	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "reuse", 5, input.Secrets, input.CommandFingerprint, input.WorkdirFingerprint)
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if !result.Reused {
		t.Fatalf("expected lease reuse")
	}

	ev, ok := findAuditEvent(audit.events, AuditEventRequestReusedActiveLease)
	if !ok {
		t.Fatalf("expected %s audit event", AuditEventRequestReusedActiveLease)
	}
	if ev.AgentID != "agent1" || ev.TaskID != "task1" {
		t.Fatalf("expected agent/task context, got agent=%q task=%q", ev.AgentID, ev.TaskID)
	}
	if ev.RequestID != "req_existing" || ev.LeaseToken != "lease_active" {
		t.Fatalf("expected request/lease context, got request=%q lease=%q", ev.RequestID, ev.LeaseToken)
	}
	if ev.Metadata["equivalence_key_hash"] == "" {
		t.Fatalf("expected equivalence_key_hash metadata")
	}
	if ev.Metadata["equivalence_key_hash"] == input.EquivalenceKey() {
		t.Fatalf("expected hashed/truncated equivalence key metadata")
	}
}

func TestRequestLeaseWithPolicyAuditsPendingCapThrottle(t *testing.T) {
	store := memory.NewStore()
	audit := &auditBuf{}
	now := time.Date(2026, 3, 11, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-20 * time.Second),
	})
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_2",
		AgentID:            "agent1",
		TaskID:             "task_2",
		Reason:             "second",
		TTLMinutes:         5,
		Secrets:            []string{"npm_token"},
		CommandFingerprint: "fp2",
		WorkdirFingerprint: "wd2",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-10 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        audit,
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task3", "third", 5, []string{"slack_token"}, "fp3", "wd3")
	if err == nil {
		t.Fatalf("expected throttle error")
	}
	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonPendingCap {
		t.Fatalf("expected pending-cap reason, got %q", throttleErr.Reason)
	}

	ev, ok := findAuditEvent(audit.events, AuditEventRequestThrottledPendingCap)
	if !ok {
		t.Fatalf("expected %s audit event", AuditEventRequestThrottledPendingCap)
	}
	if ev.Metadata["equivalence_key_hash"] == "" {
		t.Fatalf("expected equivalence_key_hash metadata")
	}
	if ev.Metadata["retry_after_seconds"] != "60" {
		t.Fatalf("expected retry_after_seconds=60, got %q", ev.Metadata["retry_after_seconds"])
	}
}

func TestRequestLeaseWithPolicyAuditsCooldownThrottle(t *testing.T) {
	store := memory.NewStore()
	audit := &auditBuf{}
	now := time.Date(2026, 3, 11, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-15 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        audit,
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task2", "repeat", 5, []string{"npm_token", "github_token"}, "fp1", "wd1")
	if err == nil {
		t.Fatalf("expected cooldown throttle error")
	}
	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonCooldown {
		t.Fatalf("expected cooldown reason, got %q", throttleErr.Reason)
	}

	ev, ok := findAuditEvent(audit.events, AuditEventRequestThrottledCooldown)
	if !ok {
		t.Fatalf("expected %s audit event", AuditEventRequestThrottledCooldown)
	}
	if ev.Metadata["equivalence_key_hash"] == "" {
		t.Fatalf("expected equivalence_key_hash metadata")
	}
	if ev.Metadata["retry_after_seconds"] != "45" {
		t.Fatalf("expected retry_after_seconds=45, got %q", ev.Metadata["retry_after_seconds"])
	}
}

func TestAuditEnvPathDecisionEvents(t *testing.T) {
	audit := &auditBuf{}
	now := time.Date(2026, 3, 11, 18, 0, 0, 0, time.UTC)
	svc := Service{Audit: audit, Now: func() time.Time { return now }}

	svc.AuditEnvPathConfirmed("agent1", "task1", "req1", "./.env", "/workspace/project/.env")
	svc.AuditEnvPathRejected("agent1", "task1", "req1", "../.env", "/workspace/project/.env", "operator_rejected")

	confirmed, ok := findAuditEvent(audit.events, AuditEventEnvPathConfirmed)
	if !ok {
		t.Fatalf("expected %s event", AuditEventEnvPathConfirmed)
	}
	if confirmed.Metadata["env_path_original"] != "./.env" {
		t.Fatalf("expected env_path_original metadata, got %q", confirmed.Metadata["env_path_original"])
	}
	if confirmed.Metadata["env_path_canonical"] != "/workspace/project/.env" {
		t.Fatalf("expected env_path_canonical metadata, got %q", confirmed.Metadata["env_path_canonical"])
	}

	rejected, ok := findAuditEvent(audit.events, AuditEventEnvPathRejected)
	if !ok {
		t.Fatalf("expected %s event", AuditEventEnvPathRejected)
	}
	if rejected.Metadata["reason"] != "operator_rejected" {
		t.Fatalf("expected reason metadata, got %q", rejected.Metadata["reason"])
	}
}

func findAuditEvent(events []ports.AuditEvent, name string) (ports.AuditEvent, bool) {
	for _, ev := range events {
		if ev.Event == name {
			return ev, true
		}
	}
	return ports.AuditEvent{}, false
}
