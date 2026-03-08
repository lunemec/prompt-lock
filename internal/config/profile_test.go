package config

import "testing"

func TestNormalizeOutputSecurityMode(t *testing.T) {
	cfg := Default()
	cfg.ExecutionPolicy.OutputSecurityMode = "weird"
	cfg.normalize()
	if cfg.ExecutionPolicy.OutputSecurityMode != "redacted" {
		t.Fatalf("expected fallback to redacted, got %q", cfg.ExecutionPolicy.OutputSecurityMode)
	}
}

func TestApplyHardenedProfile(t *testing.T) {
	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.ExecutionPolicy.MaxTimeoutSec = 600
	cfg.ExecutionPolicy.DefaultTimeoutSec = 120
	cfg.ExecutionPolicy.MaxOutputBytes = 65536
	cfg.UnixSocket = ""
	cfg.Auth.AllowPlaintextSecretReturn = true

	cfg.applyProfile()

	if cfg.Auth.AllowPlaintextSecretReturn {
		t.Fatalf("expected plaintext return disabled in hardened profile")
	}
	if cfg.ExecutionPolicy.MaxTimeoutSec > 120 {
		t.Fatalf("expected max timeout tightened")
	}
	if cfg.ExecutionPolicy.MaxOutputBytes > 32768 {
		t.Fatalf("expected max output tightened")
	}
	if cfg.ExecutionPolicy.OutputSecurityMode != "none" {
		t.Fatalf("expected hardened profile output mode none, got %q", cfg.ExecutionPolicy.OutputSecurityMode)
	}
	if cfg.UnixSocket == "" {
		t.Fatalf("expected unix socket default in hardened profile")
	}
}
