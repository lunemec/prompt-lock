package auth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestConsumeBootstrapOnce(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	s.SaveBootstrap(BootstrapToken{Token: "b1", AgentID: "a1", ContainerID: "c1", CreatedAt: now, ExpiresAt: now.Add(1 * time.Minute)})
	if _, err := s.ConsumeBootstrap("b1", "c1", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeBootstrap("b1", "c1", now); err == nil {
		t.Fatalf("expected second consume to fail")
	}
}

func TestConsumeBootstrapContainerMismatch(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	s.SaveBootstrap(BootstrapToken{Token: "b2", AgentID: "a1", ContainerID: "expected-c", CreatedAt: now, ExpiresAt: now.Add(1 * time.Minute)})
	if _, err := s.ConsumeBootstrap("b2", "wrong-c", now); err == nil {
		t.Fatalf("expected container mismatch to fail")
	}
}

func TestCleanupExpired(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	s.SaveBootstrap(BootstrapToken{Token: "b_exp", AgentID: "a", CreatedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(-1 * time.Minute)})
	s.SaveBootstrap(BootstrapToken{Token: "b_ok", AgentID: "a", CreatedAt: now, ExpiresAt: now.Add(1 * time.Minute)})
	s.SaveSession(SessionToken{Token: "s_exp", AgentID: "a", CreatedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(-1 * time.Minute)})
	s.SaveGrant(PairingGrant{GrantID: "g_exp", AgentID: "a", CreatedAt: now.Add(-3 * time.Hour), LastUsedAt: now.Add(-2 * time.Hour), IdleExpiresAt: now.Add(-1 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})

	rb, rs, rg := s.CleanupExpired(now)
	if rb < 1 || rs < 1 || rg < 1 {
		t.Fatalf("expected cleanup activity, got rb=%d rs=%d rg=%d", rb, rs, rg)
	}
}

func TestPersistAndLoadStoreMaintainsSessionAndRevocation(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "auth-store.json")

	s1 := NewStore()
	s1.SaveGrant(PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(30 * time.Minute), AbsoluteExpiresAt: now.Add(2 * time.Hour)})
	s1.SaveSession(SessionToken{Token: "sess1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)})
	if err := s1.SaveToFile(path); err != nil {
		t.Fatalf("save to file: %v", err)
	}

	s2 := NewStore()
	if err := s2.LoadFromFile(path); err != nil {
		t.Fatalf("load from file: %v", err)
	}
	if _, err := s2.ValidateSession("sess1", now.Add(1*time.Minute)); err != nil {
		t.Fatalf("expected session to remain valid after reload, got %v", err)
	}

	if err := s2.RevokeGrant("g1"); err != nil {
		t.Fatalf("revoke grant: %v", err)
	}
	if err := s2.SaveToFile(path); err != nil {
		t.Fatalf("save revoked state: %v", err)
	}

	s3 := NewStore()
	if err := s3.LoadFromFile(path); err != nil {
		t.Fatalf("reload revoked state: %v", err)
	}
	g, err := s3.GetGrant("g1")
	if err != nil {
		t.Fatalf("get grant after reload: %v", err)
	}
	if !g.Revoked {
		t.Fatalf("expected revoked grant to persist across reload")
	}
}
