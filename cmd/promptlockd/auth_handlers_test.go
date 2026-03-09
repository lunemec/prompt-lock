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
