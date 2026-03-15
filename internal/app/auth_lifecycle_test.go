package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type authAuditBuf struct {
	events []ports.AuditEvent
}

type authAuditScript struct {
	events []ports.AuditEvent
	failAt int
	calls  int
}

func (a *authAuditBuf) Write(e ports.AuditEvent) error {
	a.events = append(a.events, e)
	return nil
}

func (a *authAuditScript) Write(e ports.AuditEvent) error {
	a.calls++
	a.events = append(a.events, e)
	if a.failAt > 0 && a.calls == a.failAt {
		return errors.New("audit unavailable")
	}
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

func TestAuthLifecycleCreateBootstrapRollsBackOnAuditFailure(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	audit := &authAuditScript{failAt: 1}
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

	if _, err := svc.CreateBootstrap("a1", "c1", "operator", "token-op"); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	if got := store.Snapshot(); len(got.Bootstrap) != 0 {
		t.Fatalf("expected bootstrap rollback, got %#v", got.Bootstrap)
	}
	if persistCalls != 2 {
		t.Fatalf("expected persist + rollback persist, got %d", persistCalls)
	}
}

func TestAuthLifecycleCompletePairingRollsBackOnAuditFailure(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	setupAudit := &authAuditBuf{}
	setupSvc := AuthLifecycle{
		Store: store,
		Audit: setupAudit,
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
	boot, err := setupSvc.CreateBootstrap("a1", "c1", "operator", "token-op")
	if err != nil {
		t.Fatalf("create bootstrap: %v", err)
	}

	audit := &authAuditScript{failAt: 1}
	persistCalls := 0
	svc := AuthLifecycle{
		Store:             store,
		Audit:             audit,
		Now:               func() time.Time { return now },
		Cfg:               setupSvc.Cfg,
		NewBootstrapToken: func() string { return "boot_unused" },
		NewGrantID:        func() string { return "grant_t1" },
		NewSessionToken:   func() string { return "sess_unused" },
		Persist: func() error {
			persistCalls++
			return nil
		},
	}

	if _, err := svc.CompletePairing(boot.Token, "c1"); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	snapshot := store.Snapshot()
	if len(snapshot.Grants) != 0 {
		t.Fatalf("expected grant rollback, got %#v", snapshot.Grants)
	}
	bt := snapshot.Bootstrap[boot.Token]
	if bt.Used {
		t.Fatalf("expected bootstrap token to be restored unused")
	}
	if persistCalls != 2 {
		t.Fatalf("expected persist + rollback persist, got %d", persistCalls)
	}
}

func TestAuthLifecycleMintSessionRollsBackOnAuditFailure(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	setupSvc := AuthLifecycle{
		Store: store,
		Audit: &authAuditBuf{},
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
	boot, err := setupSvc.CreateBootstrap("a1", "c1", "operator", "token-op")
	if err != nil {
		t.Fatalf("create bootstrap: %v", err)
	}
	grant, err := setupSvc.CompletePairing(boot.Token, "c1")
	if err != nil {
		t.Fatalf("complete pairing: %v", err)
	}
	beforeGrant, err := store.GetGrant(grant.GrantID)
	if err != nil {
		t.Fatalf("get grant before mint: %v", err)
	}

	audit := &authAuditScript{failAt: 1}
	persistCalls := 0
	svc := AuthLifecycle{
		Store:             store,
		Audit:             audit,
		Now:               func() time.Time { return now },
		Cfg:               setupSvc.Cfg,
		NewBootstrapToken: func() string { return "boot_unused" },
		NewGrantID:        func() string { return "grant_unused" },
		NewSessionToken:   func() string { return "sess_t1" },
		Persist: func() error {
			persistCalls++
			return nil
		},
	}

	if _, err := svc.MintSession(grant.GrantID); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	afterGrant, err := store.GetGrant(grant.GrantID)
	if err != nil {
		t.Fatalf("get grant after mint rollback: %v", err)
	}
	if afterGrant != beforeGrant {
		t.Fatalf("expected grant rollback, before=%#v after=%#v", beforeGrant, afterGrant)
	}
	if got := store.Snapshot(); len(got.Sessions) != 0 {
		t.Fatalf("expected session rollback, got %#v", got.Sessions)
	}
	if persistCalls != 2 {
		t.Fatalf("expected persist + rollback persist, got %d", persistCalls)
	}
}

func TestAuthLifecycleRevokeRollsBackOnAuditFailure(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	setupSvc := AuthLifecycle{
		Store: store,
		Audit: &authAuditBuf{},
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
	boot, err := setupSvc.CreateBootstrap("a1", "c1", "operator", "token-op")
	if err != nil {
		t.Fatalf("create bootstrap: %v", err)
	}
	grant, err := setupSvc.CompletePairing(boot.Token, "c1")
	if err != nil {
		t.Fatalf("complete pairing: %v", err)
	}
	session, err := setupSvc.MintSession(grant.GrantID)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}

	audit := &authAuditScript{failAt: 1}
	persistCalls := 0
	svc := AuthLifecycle{
		Store:             store,
		Audit:             audit,
		Now:               func() time.Time { return now },
		Cfg:               setupSvc.Cfg,
		NewBootstrapToken: func() string { return "boot_unused" },
		NewGrantID:        func() string { return "grant_unused" },
		NewSessionToken:   func() string { return "sess_unused" },
		Persist: func() error {
			persistCalls++
			return nil
		},
	}

	if err := svc.Revoke(grant.GrantID, session.Token, "operator", "token-op"); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	grantAfter, err := store.GetGrant(grant.GrantID)
	if err != nil {
		t.Fatalf("get grant after revoke rollback: %v", err)
	}
	if grantAfter.Revoked {
		t.Fatalf("expected grant rollback to non-revoked")
	}
	sessionAfter, err := store.GetSession(session.Token)
	if err != nil {
		t.Fatalf("get session after revoke rollback: %v", err)
	}
	if sessionAfter.Revoked {
		t.Fatalf("expected session rollback to non-revoked")
	}
	if persistCalls != 2 {
		t.Fatalf("expected persist + rollback persist, got %d", persistCalls)
	}
}

func TestAuthLifecycleCreateBootstrapRePersistsRollbackWhenPersistFailsAfterDurableWrite(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	path := filepath.Join(t.TempDir(), "auth-store.json")
	persistCalls := 0
	svc := AuthLifecycle{
		Store: store,
		Audit: &authAuditBuf{},
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
			if err := store.SaveToFile(path); err != nil {
				return err
			}
			if persistCalls == 1 {
				return errors.New("parent dir sync failed after rename")
			}
			return nil
		},
	}

	if _, err := svc.CreateBootstrap("a1", "c1", "operator", "token-op"); err == nil {
		t.Fatalf("expected persist failure")
	}
	if persistCalls != 2 {
		t.Fatalf("expected persist + rollback persist, got %d", persistCalls)
	}
	if got := store.Snapshot(); len(got.Bootstrap) != 0 {
		t.Fatalf("expected in-memory bootstrap rollback, got %#v", got.Bootstrap)
	}

	reloaded := auth.NewStore()
	if err := reloaded.LoadFromFile(path); err != nil {
		t.Fatalf("reload auth store: %v", err)
	}
	if got := reloaded.Snapshot(); len(got.Bootstrap) != 0 {
		t.Fatalf("expected durable bootstrap rollback, got %#v", got.Bootstrap)
	}
}

func TestAuthLifecycleRevokeRestoresSnapshotWhenSessionTargetFails(t *testing.T) {
	now := time.Date(2026, 3, 9, 18, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	setupSvc := AuthLifecycle{
		Store: store,
		Audit: &authAuditBuf{},
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
	boot, err := setupSvc.CreateBootstrap("a1", "c1", "operator", "token-op")
	if err != nil {
		t.Fatalf("create bootstrap: %v", err)
	}
	grant, err := setupSvc.CompletePairing(boot.Token, "c1")
	if err != nil {
		t.Fatalf("complete pairing: %v", err)
	}
	session, err := setupSvc.MintSession(grant.GrantID)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}

	svc := AuthLifecycle{
		Store:             store,
		Audit:             &authAuditBuf{},
		Now:               func() time.Time { return now },
		Cfg:               setupSvc.Cfg,
		NewBootstrapToken: func() string { return "boot_unused" },
		NewGrantID:        func() string { return "grant_unused" },
		NewSessionToken:   func() string { return "sess_unused" },
	}

	if err := svc.Revoke(grant.GrantID, "missing-session", "operator", "token-op"); err == nil || err.Error() != "session not found" {
		t.Fatalf("expected session not found, got %v", err)
	}
	grantAfter, err := store.GetGrant(grant.GrantID)
	if err != nil {
		t.Fatalf("get grant after revoke rollback: %v", err)
	}
	if grantAfter.Revoked {
		t.Fatalf("expected grant rollback when session revoke fails")
	}
	sessionAfter, err := store.GetSession(session.Token)
	if err != nil {
		t.Fatalf("get session after revoke rollback: %v", err)
	}
	if sessionAfter.Revoked {
		t.Fatalf("expected session to remain active when revoke fails early")
	}
}
