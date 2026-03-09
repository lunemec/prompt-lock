package app

import (
	"errors"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type authAuditBuf struct {
	events []ports.AuditEvent
}

func (a *authAuditBuf) Write(e ports.AuditEvent) error {
	a.events = append(a.events, e)
	return nil
}

func TestAuthLifecycleFlow(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	audit := &authAuditBuf{}
	persistCalls := 0
	svc := AuthLifecycle{
		Store: store,
		Audit: audit,
		Now:   func() time.Time { return now },
		Cfg: AuthLifecycleConfig{
			BootstrapTokenTTLSeconds: 60,
			GrantIdleTimeoutMinutes:  10,
			GrantAbsoluteMaxMinutes:  120,
			SessionTTLMinutes:        15,
		},
		NewBootstrapToken: func() string { return "boot_t1" },
		NewGrantID:        func() string { return "grant_t1" },
		NewSessionToken:   func() string { return "sess_t1" },
		Persist: func() error {
			persistCalls++
			return nil
		},
	}

	boot, err := svc.CreateBootstrap("a1", "c1", "operator", "token-op")
	if err != nil {
		t.Fatalf("create bootstrap: %v", err)
	}
	if boot.Token != "boot_t1" {
		t.Fatalf("unexpected bootstrap token: %q", boot.Token)
	}

	grant, err := svc.CompletePairing(boot.Token, "c1")
	if err != nil {
		t.Fatalf("complete pairing: %v", err)
	}
	if grant.GrantID != "grant_t1" {
		t.Fatalf("unexpected grant id: %q", grant.GrantID)
	}

	sess, err := svc.MintSession(grant.GrantID)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}
	if sess.Token != "sess_t1" {
		t.Fatalf("unexpected session token: %q", sess.Token)
	}

	if err := svc.Revoke(grant.GrantID, sess.Token, "operator", "token-op"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if persistCalls != 4 {
		t.Fatalf("expected persist called 4 times, got %d", persistCalls)
	}
}

func TestAuthLifecycleCreateBootstrapReturnsPersistError(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	audit := &authAuditBuf{}
	svc := AuthLifecycle{
		Store: store,
		Audit: audit,
		Now:   func() time.Time { return now },
		Cfg: AuthLifecycleConfig{
			BootstrapTokenTTLSeconds: 60,
			GrantIdleTimeoutMinutes:  10,
			GrantAbsoluteMaxMinutes:  120,
			SessionTTLMinutes:        15,
		},
		NewBootstrapToken: func() string { return "boot_t1" },
		NewGrantID:        func() string { return "grant_t1" },
		NewSessionToken:   func() string { return "sess_t1" },
		Persist:           func() error { return errors.New("persist failed") },
	}

	if _, err := svc.CreateBootstrap("a1", "c1", "operator", "token-op"); err == nil {
		t.Fatalf("expected persist error")
	}
}

func TestAuthLifecyclePairMismatchIsAudited(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	audit := &authAuditBuf{}
	svc := AuthLifecycle{
		Store: store,
		Audit: audit,
		Now:   func() time.Time { return now },
		Cfg: AuthLifecycleConfig{
			BootstrapTokenTTLSeconds: 60,
			GrantIdleTimeoutMinutes:  10,
			GrantAbsoluteMaxMinutes:  120,
			SessionTTLMinutes:        15,
		},
		NewBootstrapToken: func() string { return "boot_t1" },
		NewGrantID:        func() string { return "grant_t1" },
		NewSessionToken:   func() string { return "sess_t1" },
	}
	if _, err := svc.CreateBootstrap("a1", "c1", "operator", "token-op"); err != nil {
		t.Fatalf("create bootstrap: %v", err)
	}

	if _, err := svc.CompletePairing("boot_t1", "wrong-c"); err == nil {
		t.Fatalf("expected mismatch error")
	}

	found := false
	for _, ev := range audit.events {
		if ev.Event == "auth_pair_denied" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected auth_pair_denied audit event")
	}
}
