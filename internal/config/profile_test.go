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
	origGOOS := configRuntimeGOOS
	configRuntimeGOOS = "darwin"
	t.Cleanup(func() { configRuntimeGOOS = origGOOS })

	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.ExecutionPolicy.MaxTimeoutSec = 600
	cfg.ExecutionPolicy.DefaultTimeoutSec = 120
	cfg.ExecutionPolicy.MaxOutputBytes = 65536
	cfg.UnixSocket = ""
	cfg.AgentUnixSocket = ""
	cfg.OperatorUnixSocket = ""
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
		for _, allowed := range cfg.ExecutionPolicy.ExactMatchExecutables {
			if allowed == disallowed {
				t.Fatalf("did not expect %q in hardened allowlist", disallowed)
			}
		}
	}
	for _, required := range []string{"go", "python", "python3", "git"} {
		found := false
		for _, allowed := range cfg.ExecutionPolicy.ExactMatchExecutables {
			if allowed == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in hardened allowlist, got %#v", required, cfg.ExecutionPolicy.ExactMatchExecutables)
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
	if cfg.AgentUnixSocket == "" || cfg.OperatorUnixSocket == "" {
		t.Fatalf("expected agent and operator unix socket defaults in hardened profile, got agent=%q operator=%q", cfg.AgentUnixSocket, cfg.OperatorUnixSocket)
	}
	if cfg.UnixSocket != "" {
		t.Fatalf("expected legacy unix socket to remain empty in dual-socket hardened default, got %q", cfg.UnixSocket)
	}
	if cfg.AgentBridgeAddress != DefaultAgentBridgeAddress {
		t.Fatalf("expected hardened profile to set default agent bridge address, got %q", cfg.AgentBridgeAddress)
	}
}

func TestApplyHardenedProfileWithNonLocalTCPDoesNotForceUnixSocket(t *testing.T) {
	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.Address = "0.0.0.0:8765"
	cfg.UnixSocket = ""
	cfg.AgentUnixSocket = ""
	cfg.OperatorUnixSocket = ""
	cfg.applyProfile()
	if cfg.UnixSocket != "" || cfg.AgentUnixSocket != "" || cfg.OperatorUnixSocket != "" {
		t.Fatalf("expected unix sockets to remain empty for non-local tcp, got legacy=%q agent=%q operator=%q", cfg.UnixSocket, cfg.AgentUnixSocket, cfg.OperatorUnixSocket)
	}
}

func TestApplyHardenedProfileWithNonIP127HostnameDoesNotForceUnixSocket(t *testing.T) {
	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.Address = "127.evil.example:8765"
	cfg.UnixSocket = ""
	cfg.AgentUnixSocket = ""
	cfg.OperatorUnixSocket = ""
	cfg.applyProfile()
	if cfg.UnixSocket != "" || cfg.AgentUnixSocket != "" || cfg.OperatorUnixSocket != "" {
		t.Fatalf("expected unix sockets to remain empty for non-IP 127.* hostname, got legacy=%q agent=%q operator=%q", cfg.UnixSocket, cfg.AgentUnixSocket, cfg.OperatorUnixSocket)
	}
}

func TestApplyHardenedProfilePreservesExplicitLegacyUnixSocket(t *testing.T) {
	cfg := Default()
	cfg.SecurityProfile = "hardened"
	cfg.UnixSocket = "/tmp/promptlock.sock"

	cfg.applyProfile()

	if cfg.UnixSocket != "/tmp/promptlock.sock" {
		t.Fatalf("expected explicit legacy unix socket to be preserved, got %q", cfg.UnixSocket)
	}
	if cfg.AgentUnixSocket != "" || cfg.OperatorUnixSocket != "" {
		t.Fatalf("expected dual sockets to remain empty in legacy single-socket mode, got agent=%q operator=%q", cfg.AgentUnixSocket, cfg.OperatorUnixSocket)
	}
}

func TestApplyHardenedProfileLeavesAgentBridgeDisabledOnLinuxByDefault(t *testing.T) {
	origGOOS := configRuntimeGOOS
	configRuntimeGOOS = "linux"
	t.Cleanup(func() { configRuntimeGOOS = origGOOS })

	cfg := Default()
	cfg.SecurityProfile = "hardened"

	cfg.applyProfile()

	if cfg.AgentBridgeAddress != "" {
		t.Fatalf("expected linux hardened default to leave agent bridge disabled, got %q", cfg.AgentBridgeAddress)
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
