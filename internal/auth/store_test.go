package auth

import (
	"testing"
	"time"
)

func TestConsumeBootstrapOnce(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	s.SaveBootstrap(BootstrapToken{Token: "b1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(1 * time.Minute)})
	if _, err := s.ConsumeBootstrap("b1", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeBootstrap("b1", now); err == nil {
		t.Fatalf("expected second consume to fail")
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
