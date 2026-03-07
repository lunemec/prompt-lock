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
