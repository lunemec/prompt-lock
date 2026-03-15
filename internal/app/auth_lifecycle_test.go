package app

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
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

func TestCreateBootstrapRollbackDoesNotClobberConcurrentBootstrap(t *testing.T) {
	now := time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	sharedLock := &sync.Mutex{}
	failingPersistStarted := make(chan struct{})
	releaseFailingPersist := make(chan struct{})
	successPersisted := make(chan struct{})
	var failingPersistOnce sync.Once
	var successPersistOnce sync.Once
	failingPersistCalls := 0

	failingSvc := AuthLifecycle{
		Store:        store,
		MutationLock: sharedLock,
		Now:          func() time.Time { return now },
		Cfg: AuthLifecycleConfig{
			BootstrapTokenTTLSeconds: 60,
			GrantIdleTimeoutMinutes:  10,
			GrantAbsoluteMaxMinutes:  120,
			SessionTTLMinutes:        15,
		},
		NewBootstrapToken: func() string { return "boot_fail" },
		NewGrantID:        func() string { return "grant_unused_fail" },
		NewSessionToken:   func() string { return "sess_unused_fail" },
		Persist: func() error {
			failingPersistCalls++
			if failingPersistCalls > 1 {
				return nil
			}
			failingPersistOnce.Do(func() { close(failingPersistStarted) })
			<-releaseFailingPersist
			return errors.New("disk full")
		},
	}

	successSvc := AuthLifecycle{
		Store:        store,
		MutationLock: sharedLock,
		Now:          func() time.Time { return now },
		Cfg: AuthLifecycleConfig{
			BootstrapTokenTTLSeconds: 60,
			GrantIdleTimeoutMinutes:  10,
			GrantAbsoluteMaxMinutes:  120,
			SessionTTLMinutes:        15,
		},
		NewBootstrapToken: func() string { return "boot_ok" },
		NewGrantID:        func() string { return "grant_unused_ok" },
		NewSessionToken:   func() string { return "sess_unused_ok" },
		Persist: func() error {
			successPersistOnce.Do(func() { close(successPersisted) })
			return nil
		},
	}

	failingErrCh := make(chan error, 1)
	go func() {
		_, err := failingSvc.CreateBootstrap("agent-fail", "container-fail", "operator", "token-op")
		failingErrCh <- err
	}()

	<-failingPersistStarted

	successErrCh := make(chan error, 1)
	go func() {
		_, err := successSvc.CreateBootstrap("agent-ok", "container-ok", "operator", "token-op")
		successErrCh <- err
	}()

	select {
	case <-successPersisted:
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFailingPersist)

	if err := <-failingErrCh; err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected failing bootstrap persist error, got %v", err)
	}
	if err := <-successErrCh; err != nil {
		t.Fatalf("expected concurrent bootstrap to succeed, got %v", err)
	}

	snapshot := store.Snapshot()
	if _, ok := snapshot.Bootstrap["boot_ok"]; !ok {
		t.Fatalf("expected successful bootstrap to remain after rollback")
	}
	if _, ok := snapshot.Bootstrap["boot_fail"]; ok {
		t.Fatalf("expected failed bootstrap to be removed during rollback")
	}
}

func TestAuthLifecycleAuditMetadataHashesBearerCredentials(t *testing.T) {
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
		NewGrantID:        func() string { return "grant_sensitive_value" },
		NewSessionToken:   func() string { return "sess_sensitive_value" },
		Persist:           func() error { return nil },
	}

	boot, err := svc.CreateBootstrap("a1", "c1", "operator", "token-op")
	if err != nil {
		t.Fatalf("create bootstrap: %v", err)
	}
	grant, err := svc.CompletePairing(boot.Token, "c1")
	if err != nil {
		t.Fatalf("complete pairing: %v", err)
	}
	session, err := svc.MintSession(grant.GrantID)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}
	if err := svc.Revoke(grant.GrantID, session.Token, "operator", "token-op"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	pairCompleted := mustFindAuthAuditEvent(t, audit.events, "auth_pair_completed")
	if got, want := pairCompleted.Metadata["grant_id"], hashedAuditCredentialRef(grant.GrantID); got != want {
		t.Fatalf("auth_pair_completed grant_id = %q, want %q", got, want)
	}
	if got := pairCompleted.Metadata["container_id"]; got != "c1" {
		t.Fatalf("auth_pair_completed container_id = %q, want c1", got)
	}

	sessionMinted := mustFindAuthAuditEvent(t, audit.events, "auth_session_minted")
	if got, want := sessionMinted.Metadata["grant_id"], hashedAuditCredentialRef(grant.GrantID); got != want {
		t.Fatalf("auth_session_minted grant_id = %q, want %q", got, want)
	}

	revoked := mustFindAuthAuditEvent(t, audit.events, "auth_revoked")
	if got, want := revoked.Metadata["grant_id"], hashedAuditCredentialRef(grant.GrantID); got != want {
		t.Fatalf("auth_revoked grant_id = %q, want %q", got, want)
	}
	if got, want := revoked.Metadata["session_token"], hashedAuditCredentialRef(session.Token); got != want {
		t.Fatalf("auth_revoked session_token = %q, want %q", got, want)
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

func mustFindAuthAuditEvent(t *testing.T, events []ports.AuditEvent, name string) ports.AuditEvent {
	t.Helper()
	for _, event := range events {
		if event.Event == name {
			return event
		}
	}
	t.Fatalf("expected audit event %q", name)
	return ports.AuditEvent{}
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
