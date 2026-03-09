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

func TestApplyHardenedProfileWithTLSEnabledDoesNotForceUnixSocket(t *testing.T) {
	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.TLS.Enable = true
	cfg.TLS.CertFile = "/tmp/cert.pem"
	cfg.TLS.KeyFile = "/tmp/key.pem"
	cfg.UnixSocket = ""

	cfg.applyProfile()

	if cfg.UnixSocket != "" {
		t.Fatalf("expected unix socket to remain empty when tls is enabled, got %q", cfg.UnixSocket)
	}
}

func TestApplyHardenedProfileWithNonLocalTCPDoesNotForceUnixSocket(t *testing.T) {
	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.Address = "0.0.0.0:8765"
	cfg.UnixSocket = ""
	cfg.applyProfile()
	if cfg.UnixSocket != "" {
		t.Fatalf("expected unix socket to remain empty for non-local tcp, got %q", cfg.UnixSocket)
	}
}

func TestApplyHardenedProfileWithNonIP127HostnameDoesNotForceUnixSocket(t *testing.T) {
	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.Address = "127.evil.example:8765"
	cfg.UnixSocket = ""
	cfg.applyProfile()
	if cfg.UnixSocket != "" {
		t.Fatalf("expected unix socket to remain empty for non-IP 127.* hostname, got %q", cfg.UnixSocket)
	}
}

func TestIsLocalAddressConfig(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8765":             true,
		"127.0.0.2:8765":             true,
		"localhost:8765":             true,
		"localhost":                  true,
		"[::1]:8765":                 true,
		"::1":                        true,
		"127.evil.example:8765":      false,
		"127.localhost.invalid:8765": false,
		"0.0.0.0:8765":               false,
		"10.0.0.5:8765":              false,
	}
	for in, want := range cases {
		if got := isLocalAddressConfig(in); got != want {
			t.Fatalf("isLocalAddressConfig(%q)=%v want %v", in, got, want)
		}
	}
}
