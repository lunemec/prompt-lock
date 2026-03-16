package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type rlBucket struct {
	count   int
	resetAt time.Time
}

type authRateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	max     int
	buckets map[string]rlBucket
	enabled bool
}

func newAuthRateLimiter(cfg config.AuthConfig) *authRateLimiter {
	w := cfg.RateLimitWindowSeconds
	m := cfg.RateLimitMaxAttempts
	if w <= 0 {
		w = 60
	}
	if m <= 0 {
		m = 20
	}
	return &authRateLimiter{window: time.Duration(w) * time.Second, max: m, buckets: map[string]rlBucket{}, enabled: true}
}

func (l *authRateLimiter) key(r *http.Request, scope string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil || host == "" {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	if host == "" {
		host = "unknown"
	}
	return scope + ":" + strings.ToLower(host)
}

func (l *authRateLimiter) allow(key string, now time.Time) bool {
	if l == nil || !l.enabled {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	if b.resetAt.IsZero() || !now.Before(b.resetAt) {
		b = rlBucket{count: 0, resetAt: now.Add(l.window)}
	}
	if b.count >= l.max {
		l.buckets[key] = b
		return false
	}
	l.buckets[key] = b
	return true
}

func (l *authRateLimiter) recordFailure(key string, now time.Time) bool {
	if l == nil || !l.enabled {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	if b.resetAt.IsZero() || !now.Before(b.resetAt) {
		b = rlBucket{count: 0, resetAt: now.Add(l.window)}
	}
	b.count++
	l.buckets[key] = b
	return b.count >= l.max
}

func (s *server) enforceAuthRateLimit(w http.ResponseWriter, r *http.Request, scope string) bool {
	if s.authLimiter == nil {
		return true
	}
	key := s.authLimiter.key(r, scope)
	if s.authLimiter.allow(key, s.now()) {
		return true
	}
	_ = s.svc.Audit.Write(ports.AuditEvent{Event: "auth_rate_limited", Timestamp: s.now(), ActorType: scope, ActorID: key})
	writeMappedError(w, ErrRateLimited, "too many auth attempts")
	return false
}

func (s *server) recordAuthFailure(r *http.Request, scope, reason string) {
	if s.authLimiter == nil {
		return
	}
	key := s.authLimiter.key(r, scope)
	tripped := s.authLimiter.recordFailure(key, s.now())
	if tripped {
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "auth_rate_limit_threshold", Timestamp: s.now(), ActorType: scope, ActorID: key, Metadata: map[string]string{"reason": reason}})
	}
}
