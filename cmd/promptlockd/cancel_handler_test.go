package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestHandleCancelOwnPendingRequest(t *testing.T) {
	s := testServer(time.Now().UTC())
	created, err := s.svc.RequestLease("a1", "task-1", "test", 5, []string{"github_token"}, "fp-cancel", "wd-cancel", "", "")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id="+created.ID, bytes.NewBufferString(`{"reason":"mcp cancelled"}`))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	cancelled, err := s.svc.Requests.GetRequest(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != domain.RequestDenied {
		t.Fatalf("expected denied status after cancel, got %s", cancelled.Status)
	}
}

func TestHandleCancelRejectsNonOwner(t *testing.T) {
	s := testServer(time.Now().UTC())
	created, err := s.svc.RequestLease("a-other", "task-1", "test", 5, []string{"github_token"}, "fp-cancel", "wd-cancel", "", "")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id="+created.ID, bytes.NewBufferString(`{"reason":"mcp cancelled"}`))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCancelNoAuthAllowsCancellation(t *testing.T) {
	s := testServer(time.Now().UTC())
	s.authEnabled = false
	created, err := s.svc.RequestLease("a-other", "task-1", "test", 5, []string{"github_token"}, "fp-cancel", "wd-cancel", "", "")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id="+created.ID, bytes.NewBufferString(`{"reason":"mcp cancelled"}`))
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	cancelled, err := s.svc.Requests.GetRequest(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != domain.RequestDenied {
		t.Fatalf("expected denied status after cancel, got %s", cancelled.Status)
	}
}
