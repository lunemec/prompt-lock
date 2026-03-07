package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestIsLocalAddress(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8765": true,
		"localhost:8765": true,
		"0.0.0.0:8765":   false,
		"10.0.0.5:8765":  false,
	}
	for in, want := range cases {
		if got := isLocalAddress(in); got != want {
			t.Fatalf("isLocalAddress(%q)=%v want %v", in, got, want)
		}
	}
}

func TestValidateTransportSafety(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.EnableAuth = true
	cfg.Address = "0.0.0.0:8765"
	cfg.UnixSocket = ""
	if err := validateTransportSafety(cfg, ""); err == nil {
		t.Fatalf("expected transport safety error")
	}
	if err := validateTransportSafety(cfg, "1"); err != nil {
		t.Fatalf("expected override success, got %v", err)
	}
	cfg.UnixSocket = "/tmp/promptlock.sock"
	if err := validateTransportSafety(cfg, ""); err != nil {
		t.Fatalf("expected unix socket to satisfy safety, got %v", err)
	}
}
