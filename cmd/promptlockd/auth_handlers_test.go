package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type nopAudit struct{}

func (nopAudit) Write(event ports.AuditEvent) error { return nil }

type failingAuthStorePersister struct{}

func (failingAuthStorePersister) SaveToFile(string) error { return errors.New("disk full") }
func (failingAuthStorePersister) SaveToFileEncrypted(string, []byte) error {
	return errors.New("disk full")
}

func testAuthServer() *server {
	return &server{
		svc:         app.Service{Audit: nopAudit{}, Now: func() time.Time { return time.Now().UTC() }},
		authEnabled: true,
		authCfg: config.AuthConfig{
			EnableAuth:               true,
			OperatorToken:            "op-token",
			BootstrapTokenTTLSeconds: 60,
			GrantIdleTimeoutMinutes:  10,
			GrantAbsoluteMaxMinutes:  60,
			SessionTTLMinutes:        10,
		},
		authStore: auth.NewStore(),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func TestPairCompleteRejectsContainerMismatch(t *testing.T) {
	s := testAuthServer()

	// Create bootstrap token bound to container c1
	createBody := bytes.NewBufferString(`{"agent_id":"a1","container_id":"c1"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/auth/bootstrap/create", createBody)
	createReq.Header.Set("Authorization", "Bearer op-token")
	createW := httptest.NewRecorder()
	s.handleAuthBootstrapCreate(createW, createReq)
	if createW.Code != http.StatusOK {
		t.Fatalf("bootstrap create failed: code=%d body=%s", createW.Code, createW.Body.String())
	}
	var created struct {
		BootstrapToken string `json:"bootstrap_token"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if created.BootstrapToken == "" {
		t.Fatalf("expected bootstrap token")
	}

	// Pair with wrong container id should fail
	pairPayload := map[string]string{"token": created.BootstrapToken, "container_id": "c2"}
	b, _ := json.Marshal(pairPayload)
	pairReq := httptest.NewRequest(http.MethodPost, "/v1/auth/pair/complete", bytes.NewReader(b))
	pairW := httptest.NewRecorder()
	s.handleAuthPairComplete(pairW, pairReq)
	if pairW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on container mismatch, got %d body=%s", pairW.Code, pairW.Body.String())
	}
}

func TestAuthBootstrapCreateFailsClosedOnPersistFailure(t *testing.T) {
	s := testAuthServer()
	s.authStoreFile = "/tmp/promptlock-auth-store.json"
	s.authStorePersister = failingAuthStorePersister{}

	createBody := bytes.NewBufferString(`{"agent_id":"a1","container_id":"c1"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/auth/bootstrap/create", createBody)
	createReq.Header.Set("Authorization", "Bearer op-token")
	createW := httptest.NewRecorder()
	s.handleAuthBootstrapCreate(createW, createReq)
	if createW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on persist failure, got %d body=%s", createW.Code, createW.Body.String())
	}
	if !strings.Contains(createW.Body.String(), "durability persistence unavailable") {
		t.Fatalf("expected durability error message, got %q", createW.Body.String())
	}
}

func TestAuthRevokeRequiresExplicitTarget(t *testing.T) {
	s := testAuthServer()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/revoke", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()

	s.handleAuthRevoke(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when revoke target missing, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthRevokeAcceptsCanonicalSessionTokenField(t *testing.T) {
	s := testAuthServer()
	now := time.Now().UTC()
	s.authStore.SaveGrant(auth.PairingGrant{
		GrantID:           "grant-1",
		AgentID:           "agent-1",
		ContainerID:       "container-1",
		CreatedAt:         now,
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(10 * time.Minute),
		AbsoluteExpiresAt: now.Add(20 * time.Minute),
	})
	s.authStore.SaveSession(auth.SessionToken{
		Token:     "sess-1",
		GrantID:   "grant-1",
		AgentID:   "agent-1",
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/revoke", bytes.NewBufferString(`{"session_token":"sess-1"}`))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()

	s.handleAuthRevoke(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected session_token revoke success, got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := s.authStore.ValidateSession("sess-1", now.Add(time.Minute)); err == nil {
		t.Fatalf("expected session token revoked")
	}
}

func TestPairCompleteRateLimitedAfterRepeatedFailures(t *testing.T) {
	s := testAuthServer()
	s.authLimiter = &authRateLimiter{
		window:  time.Minute,
		max:     1,
		buckets: map[string]rlBucket{},
		enabled: true,
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/auth/pair/complete", bytes.NewBufferString(`{"token":"","container_id":"c1"}`))
	firstReq.RemoteAddr = "127.0.0.1:1234"
	firstW := httptest.NewRecorder()
	s.handleAuthPairComplete(firstW, firstReq)
	if firstW.Code != http.StatusBadRequest {
		t.Fatalf("expected first bad pair request to be rejected as bad request, got %d body=%s", firstW.Code, firstW.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/auth/pair/complete", bytes.NewBufferString(`{"token":"","container_id":"c1"}`))
	secondReq.RemoteAddr = "127.0.0.1:1234"
	secondW := httptest.NewRecorder()
	s.handleAuthPairComplete(secondW, secondReq)
	if secondW.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second bad pair request to be rate limited, got %d body=%s", secondW.Code, secondW.Body.String())
	}
}

func TestSessionMintRateLimitedAfterRepeatedFailures(t *testing.T) {
	s := testAuthServer()
	s.authLimiter = &authRateLimiter{
		window:  time.Minute,
		max:     1,
		buckets: map[string]rlBucket{},
		enabled: true,
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/auth/session/mint", bytes.NewBufferString(`{"grant_id":"missing"}`))
	firstReq.RemoteAddr = "127.0.0.1:2234"
	firstW := httptest.NewRecorder()
	s.handleAuthSessionMint(firstW, firstReq)
	if firstW.Code != http.StatusNotFound {
		t.Fatalf("expected first bad mint request to be rejected as not found, got %d body=%s", firstW.Code, firstW.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/auth/session/mint", bytes.NewBufferString(`{"grant_id":"missing"}`))
	secondReq.RemoteAddr = "127.0.0.1:2234"
	secondW := httptest.NewRecorder()
	s.handleAuthSessionMint(secondW, secondReq)
	if secondW.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second bad mint request to be rate limited, got %d body=%s", secondW.Code, secondW.Body.String())
	}
}

func TestAuthRevokeRequiresTarget(t *testing.T) {
	s := testAuthServer()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/revoke", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	s.handleAuthRevoke(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when revoke target is omitted, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthPairCompleteEnforcesRateLimit(t *testing.T) {
	s := testAuthServer()
	s.authLimiter = &authRateLimiter{enabled: true, window: time.Minute, max: 0, buckets: map[string]rlBucket{}}

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/pair/complete", bytes.NewBufferString(`{"token":"boot_1","container_id":"c1"}`))
	w := httptest.NewRecorder()
	s.handleAuthPairComplete(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected pair-complete to be rate limited, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthSessionMintEnforcesRateLimit(t *testing.T) {
	s := testAuthServer()
	s.authLimiter = &authRateLimiter{enabled: true, window: time.Minute, max: 0, buckets: map[string]rlBucket{}}

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/session/mint", bytes.NewBufferString(`{"grant_id":"grant_1"}`))
	w := httptest.NewRecorder()
	s.handleAuthSessionMint(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected session-mint to be rate limited, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthRevokeAcceptsSessionTokenField(t *testing.T) {
	now := time.Now().UTC()
	s := testAuthServer()
	s.authStore.SaveGrant(auth.PairingGrant{
		GrantID:           "grant-1",
		AgentID:           "agent-1",
		CreatedAt:         now,
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(10 * time.Minute),
		AbsoluteExpiresAt: now.Add(1 * time.Hour),
	})
	s.authStore.SaveSession(auth.SessionToken{
		Token:     "sess-1",
		GrantID:   "grant-1",
		AgentID:   "agent-1",
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/revoke", bytes.NewBufferString(`{"session_token":"sess-1"}`))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	s.handleAuthRevoke(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected revoke by session_token to succeed, got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := s.authStore.ValidateSession("sess-1", now.Add(time.Minute)); err == nil {
		t.Fatalf("expected session_token revoke to revoke the session")
	}
}
