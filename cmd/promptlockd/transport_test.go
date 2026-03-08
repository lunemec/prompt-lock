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
	cfg.UnixSocket = ""
	cfg.TLS.Enable = true
	cfg.TLS.CertFile = "/tmp/cert.pem"
	cfg.TLS.KeyFile = "/tmp/key.pem"
	if err := validateTransportSafety(cfg, ""); err != nil {
		t.Fatalf("expected tls to satisfy safety, got %v", err)
	}
}

func TestValidateTransportSafetyWithTLS(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.EnableAuth = true
	cfg.Address = "0.0.0.0:8765"
	cfg.UnixSocket = ""
	cfg.TLS.Enable = true
	cfg.TLS.CertFile = "/tmp/cert.pem"
	cfg.TLS.KeyFile = "/tmp/key.pem"
	if err := validateTransportSafety(cfg, ""); err != nil {
		t.Fatalf("expected tls transport safety success, got %v", err)
	}
}

func TestValidateSecretSourceSafety(t *testing.T) {
	cfg := config.Default()
	cfg.SecurityProfile = "hardened"
	cfg.SecretSource.Type = "in_memory"
	cfg.SecretSource.InMemoryHardened = "warn"
	if err := validateSecretSourceSafety(cfg); err != nil {
		t.Fatalf("expected warn mode to pass, got %v", err)
	}
	cfg.SecretSource.InMemoryHardened = "fail"
	if err := validateSecretSourceSafety(cfg); err == nil {
		t.Fatalf("expected fail mode to reject in_memory source in hardened")
	}
	cfg.SecretSource.Type = "env"
	if err := validateSecretSourceSafety(cfg); err != nil {
		t.Fatalf("expected env source to pass in hardened, got %v", err)
	}
}

func TestValidateTLSConfig(t *testing.T) {
	cfg := config.Default()
	cfg.TLS.Enable = true
	if err := validateTLSConfig(cfg); err == nil {
		t.Fatalf("expected error for missing cert/key")
	}
	cfg.TLS.CertFile = "/tmp/cert.pem"
	cfg.TLS.KeyFile = "/tmp/key.pem"
	if err := validateTLSConfig(cfg); err != nil {
		t.Fatalf("expected tls config success, got %v", err)
	}
	cfg.TLS.RequireClientCert = true
	cfg.TLS.ClientCAFile = ""
	if err := validateTLSConfig(cfg); err == nil {
		t.Fatalf("expected error for missing client CA in mTLS mode")
	}
}
