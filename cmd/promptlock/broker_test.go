package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResponseErrorSanitizesBrokerBody(t *testing.T) {
	resp := &http.Response{
		Status:     "500 Internal Server Error",
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader("broker said:\x1b[31mnope\x1b[0m")),
	}

	err := responseError("request failed", resp)
	if err == nil {
		t.Fatal("expected responseError to return an error")
	}
	got := err.Error()
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected response error to sanitize escape sequences, got %q", got)
	}
	if !strings.Contains(got, `broker said:\x1b[31mnope\x1b[0m`) {
		t.Fatalf("expected sanitized broker body in error, got %q", got)
	}
	if !strings.Contains(got, "500 Internal Server Error") {
		t.Fatalf("expected status text in error, got %q", got)
	}
}
