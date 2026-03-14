package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestApproveRejectsMalformedJSON(t *testing.T) {
	s := testServer(time.Now().UTC())
	created, err := s.svc.RequestLease("a1", "task-1", "test", 5, []string{"github_token"}, "fp-approve", "wd-approve", "", "")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id="+created.ID, bytes.NewBufferString(`{"ttl_minutes":`))
	req.Header.Set("Authorization", "Bearer op")
	w := httptest.NewRecorder()
	s.handleApprove(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}

	stored, err := s.svc.Requests.GetRequest(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected malformed approve body to leave request pending, got %s", stored.Status)
	}
}

func TestDenyRejectsMalformedJSON(t *testing.T) {
	s := testServer(time.Now().UTC())
	created, err := s.svc.RequestLease("a1", "task-1", "test", 5, []string{"github_token"}, "fp-deny", "wd-deny", "", "")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/deny?request_id="+created.ID, bytes.NewBufferString(`{"reason":`))
	req.Header.Set("Authorization", "Bearer op")
	w := httptest.NewRecorder()
	s.handleDeny(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}

	stored, err := s.svc.Requests.GetRequest(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected malformed deny body to leave request pending, got %s", stored.Status)
	}
}

func TestCancelRejectsMalformedJSON(t *testing.T) {
	s := testServer(time.Now().UTC())
	created, err := s.svc.RequestLease("a1", "task-1", "test", 5, []string{"github_token"}, "fp-cancel", "wd-cancel", "", "")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id="+created.ID, bytes.NewBufferString(`{"reason":`))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}

	stored, err := s.svc.Requests.GetRequest(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected malformed cancel body to leave request pending, got %s", stored.Status)
	}
}
