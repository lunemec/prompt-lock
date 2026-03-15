package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
)

type persistThenFailAuthStorePersister struct {
	store  *auth.Store
	failAt int
	calls  int
}

func (p *persistThenFailAuthStorePersister) SaveToFile(path string) error {
	p.calls++
	if err := p.store.SaveToFile(path); err != nil {
		return err
	}
	if p.failAt > 0 && p.calls == p.failAt {
		return errors.New("parent dir sync failed after rename")
	}
	return nil
}

func (p *persistThenFailAuthStorePersister) SaveToFileEncrypted(path string, key []byte) error {
	p.calls++
	if err := p.store.SaveToFileEncrypted(path, key); err != nil {
		return err
	}
	if p.failAt > 0 && p.calls == p.failAt {
		return errors.New("parent dir sync failed after rename")
	}
	return nil
}

func TestRunAuthCleanupPassRollsBackWhenAuditFails(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	store.SaveBootstrap(auth.BootstrapToken{
		Token:       "boot-expired",
		AgentID:     "a1",
		ContainerID: "c1",
		CreatedAt:   now.Add(-2 * time.Hour),
		ExpiresAt:   now.Add(-time.Hour),
	})
	store.SaveGrant(auth.PairingGrant{
		GrantID:           "grant-expired",
		AgentID:           "a1",
		ContainerID:       "c1",
		CreatedAt:         now.Add(-2 * time.Hour),
		LastUsedAt:        now.Add(-2 * time.Hour),
		IdleExpiresAt:     now.Add(-time.Hour),
		AbsoluteExpiresAt: now.Add(30 * time.Minute),
	})
	store.SaveSession(auth.SessionToken{
		Token:     "sess-expired",
		GrantID:   "grant-expired",
		AgentID:   "a1",
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
	})

	authFile := filepath.Join(t.TempDir(), "auth-store.json")
	s := &server{
		svc:                app.Service{Audit: failingAudit{}},
		authEnabled:        true,
		authStore:          store,
		authStorePersister: store,
		authStoreFile:      authFile,
		now:                func() time.Time { return now },
	}

	if err := runAuthCleanupPass(s); !errors.Is(err, ErrDurabilityClosed) {
		t.Fatalf("expected durability-closed error, got %v", err)
	}

	snapshot := store.Snapshot()
	if _, ok := snapshot.Bootstrap["boot-expired"]; !ok {
		t.Fatalf("expected bootstrap rollback after cleanup audit failure")
	}
	grant := snapshot.Grants["grant-expired"]
	if grant.Revoked {
		t.Fatalf("expected grant rollback after cleanup audit failure")
	}
	if _, ok := snapshot.Sessions["sess-expired"]; !ok {
		t.Fatalf("expected session rollback after cleanup audit failure")
	}

	reloaded := auth.NewStore()
	if err := reloaded.LoadFromFile(authFile); err != nil {
		t.Fatalf("reload auth store: %v", err)
	}
	reloadedSnapshot := reloaded.Snapshot()
	if _, ok := reloadedSnapshot.Bootstrap["boot-expired"]; !ok {
		t.Fatalf("expected durable bootstrap rollback after cleanup audit failure")
	}
	if reloadedSnapshot.Grants["grant-expired"].Revoked {
		t.Fatalf("expected durable grant rollback after cleanup audit failure")
	}
	if _, ok := reloadedSnapshot.Sessions["sess-expired"]; !ok {
		t.Fatalf("expected durable session rollback after cleanup audit failure")
	}
}

func TestRunAuthCleanupPassRePersistsRollbackWhenPersistFailsAfterDurableWrite(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	store := auth.NewStore()
	store.SaveBootstrap(auth.BootstrapToken{
		Token:       "boot-expired",
		AgentID:     "a1",
		ContainerID: "c1",
		CreatedAt:   now.Add(-2 * time.Hour),
		ExpiresAt:   now.Add(-time.Hour),
	})
	store.SaveGrant(auth.PairingGrant{
		GrantID:           "grant-expired",
		AgentID:           "a1",
		ContainerID:       "c1",
		CreatedAt:         now.Add(-2 * time.Hour),
		LastUsedAt:        now.Add(-2 * time.Hour),
		IdleExpiresAt:     now.Add(-time.Hour),
		AbsoluteExpiresAt: now.Add(30 * time.Minute),
	})
	store.SaveSession(auth.SessionToken{
		Token:     "sess-expired",
		GrantID:   "grant-expired",
		AgentID:   "a1",
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
	})

	authFile := filepath.Join(t.TempDir(), "auth-store.json")
	if err := store.SaveToFile(authFile); err != nil {
		t.Fatalf("seed auth store: %v", err)
	}
	persister := &persistThenFailAuthStorePersister{store: store, failAt: 1}
	s := &server{
		svc:                app.Service{Audit: testAudit{}},
		authEnabled:        true,
		authStore:          store,
		authStorePersister: persister,
		authStoreFile:      authFile,
		now:                func() time.Time { return now },
	}

	if err := runAuthCleanupPass(s); !errors.Is(err, ErrDurabilityClosed) {
		t.Fatalf("expected durability-closed error, got %v", err)
	}
	if persister.calls != 2 {
		t.Fatalf("expected persist + rollback persist, got %d calls", persister.calls)
	}

	snapshot := store.Snapshot()
	if _, ok := snapshot.Bootstrap["boot-expired"]; !ok {
		t.Fatalf("expected bootstrap rollback after cleanup persist failure")
	}
	if snapshot.Grants["grant-expired"].Revoked {
		t.Fatalf("expected grant rollback after cleanup persist failure")
	}
	if _, ok := snapshot.Sessions["sess-expired"]; !ok {
		t.Fatalf("expected session rollback after cleanup persist failure")
	}

	reloaded := auth.NewStore()
	if err := reloaded.LoadFromFile(authFile); err != nil {
		t.Fatalf("reload auth store: %v", err)
	}
	reloadedSnapshot := reloaded.Snapshot()
	if _, ok := reloadedSnapshot.Bootstrap["boot-expired"]; !ok {
		t.Fatalf("expected durable bootstrap rollback after cleanup persist failure")
	}
	if reloadedSnapshot.Grants["grant-expired"].Revoked {
		t.Fatalf("expected durable grant rollback after cleanup persist failure")
	}
	if _, ok := reloadedSnapshot.Sessions["sess-expired"]; !ok {
		t.Fatalf("expected durable session rollback after cleanup persist failure")
	}
}
