package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
)

type persistShouldNotBeCalled struct{}

func (persistShouldNotBeCalled) SaveStateToFile(string) error {
	return errors.New("persist should not be called for reused lease response")
}

func TestHandleRequestReturnsReusedLeaseWithoutPersistingState(t *testing.T) {
	now := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.SaveLease(domain.Lease{
		Token:              "lease-existing",
		RequestID:          "req-existing",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp-1",
		WorkdirFingerprint: "wd-1",
		ExpiresAt:          now.Add(10 * time.Minute),
	})

	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-new" },
			NewLeaseTok:  func() string { return "lease-new" },
		},
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: persistShouldNotBeCalled{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"agent-1","task_id":"task-2","reason":"repeat","ttl_minutes":5,"secrets":["npm_token"," github_token "],"command_fingerprint":"fp-1","workdir_fingerprint":"wd-1"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for reused lease, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"reused"`) {
		t.Fatalf("expected reused status in response, got %s", body)
	}
	if !strings.Contains(body, `"lease_token":"lease-existing"`) {
		t.Fatalf("expected reused lease token in response, got %s", body)
	}
	if !strings.Contains(body, `"request_id":"req-existing"`) {
		t.Fatalf("expected reused response to include original request id, got %s", body)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no new pending requests, got %d", len(pending))
	}
}
