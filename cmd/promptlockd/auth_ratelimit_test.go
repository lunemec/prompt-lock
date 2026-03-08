package main

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/config"
)

func TestAuthRateLimiterThresholdAndRecovery(t *testing.T) {
	cfg := config.AuthConfig{RateLimitWindowSeconds: 1, RateLimitMaxAttempts: 2}
	rl := newAuthRateLimiter(cfg)
	now := time.Now().UTC()
	key := "agent:127.0.0.1"

	if !rl.allow(key, now) {
		t.Fatalf("first allow should pass")
	}
	if tripped := rl.recordFailure(key, now); tripped {
		t.Fatalf("first failure should not trip threshold")
	}
	if tripped := rl.recordFailure(key, now); !tripped {
		t.Fatalf("second failure should trip threshold")
	}
	if rl.allow(key, now) {
		t.Fatalf("allow should fail once threshold reached")
	}
	if !rl.allow(key, now.Add(2*time.Second)) {
		t.Fatalf("allow should recover after window reset")
	}
}

func TestAuthRateLimiterKeyDerivation(t *testing.T) {
	rl := newAuthRateLimiter(config.AuthConfig{})
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	k := rl.key(r, "operator")
	if k != "operator:10.0.0.5" {
		t.Fatalf("unexpected key: %s", k)
	}
}
