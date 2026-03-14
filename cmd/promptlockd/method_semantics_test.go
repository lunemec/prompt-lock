package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecuteMethodMismatchReturns405(t *testing.T) {
	s := &server{authEnabled: false}
	req := httptest.NewRequest(http.MethodGet, "/v1/leases/execute", nil)
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHostDockerMethodMismatchReturns405(t *testing.T) {
	s := &server{authEnabled: false}
	req := httptest.NewRequest(http.MethodGet, "/v1/host/docker/execute", nil)
	w := httptest.NewRecorder()
	s.handleHostDockerExecute(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestCancelMethodMismatchReturns405(t *testing.T) {
	s := &server{authEnabled: false}
	req := httptest.NewRequest(http.MethodGet, "/v1/leases/cancel?request_id=req-1", nil)
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
