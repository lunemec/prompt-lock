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
	for _, disallowed := range []string{"bash", "sh", "zsh"} {
		for _, allowed := range cfg.ExecutionPolicy.AllowlistPrefixes {
			if allowed == disallowed {
				t.Fatalf("did not expect %q in hardened allowlist", disallowed)
			}
		}
	}
	if len(cfg.HostOpsPolicy.DockerComposeAllowVerbs) != 2 || cfg.HostOpsPolicy.DockerComposeAllowVerbs[0] != "config" || cfg.HostOpsPolicy.DockerComposeAllowVerbs[1] != "ps" {
		t.Fatalf("expected hardened compose verbs to be [config ps], got %#v", cfg.HostOpsPolicy.DockerComposeAllowVerbs)
	}
	foundSmuggleGuard := false
	for _, d := range cfg.ExecutionPolicy.DenylistSubstrings {
		if d == "&&" {
			foundSmuggleGuard = true
			break
		}
	}
	if !foundSmuggleGuard {
		t.Fatalf("expected hardened denylist to include command-smuggling guards")
	}
	if cfg.UnixSocket == "" {
		t.Fatalf("expected unix socket default in hardened profile")
	}
}
