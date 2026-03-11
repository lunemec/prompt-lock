package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Policy.DefaultTTLMinutes != 5 {
		t.Fatalf("expected default ttl 5")
	}
}

func TestLoadFromFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	data := `{"address":":9999","tls":{"enable":true,"cert_file":"/tmp/cert.pem","key_file":"/tmp/key.pem"},"policy":{"default_ttl_minutes":7,"min_ttl_minutes":1,"max_ttl_minutes":20,"max_secrets_per_request":3}}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != ":9999" || cfg.Policy.DefaultTTLMinutes != 7 || cfg.Policy.MaxTTLMinutes != 20 {
		t.Fatalf("config values not loaded correctly: %+v", cfg)
	}
	if !cfg.TLS.Enable || cfg.TLS.CertFile != "/tmp/cert.pem" || cfg.TLS.KeyFile != "/tmp/key.pem" {
		t.Fatalf("tls values not loaded correctly: %+v", cfg.TLS)
	}
}

func TestSecretSourceDefaultsNormalize(t *testing.T) {
	cfg := Default()
	cfg.SecretSource = SecretSourceConfig{}
	cfg.Auth.StoreEncryptionKeyEnv = ""
	cfg.normalize()
	if cfg.SecretSource.Type != "in_memory" {
		t.Fatalf("expected in_memory default, got %q", cfg.SecretSource.Type)
	}
	if cfg.SecretSource.EnvPrefix == "" {
		t.Fatalf("expected default env prefix")
	}
	if cfg.SecretSource.InMemoryHardened != "warn" {
		t.Fatalf("expected warn default, got %q", cfg.SecretSource.InMemoryHardened)
	}
	if cfg.SecretSource.ExternalAuthTokenEnv != "PROMPTLOCK_EXTERNAL_SECRET_TOKEN" {
		t.Fatalf("expected default external token env, got %q", cfg.SecretSource.ExternalAuthTokenEnv)
	}
	if cfg.SecretSource.ExternalTimeoutSec <= 0 {
		t.Fatalf("expected positive external timeout default")
	}
	if cfg.Auth.StoreEncryptionKeyEnv != "PROMPTLOCK_AUTH_STORE_KEY" {
		t.Fatalf("expected default auth store encryption key env, got %q", cfg.Auth.StoreEncryptionKeyEnv)
	}
}

func TestLoadHardenedProfileWithNonIP127HostnameDoesNotDefaultUnixSocket(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg-hardened-nonlocal.json")
	data := `{"security_profile":"hardened","address":"127.evil.example:8765","unix_socket":""}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnixSocket != "" {
		t.Fatalf("expected unix socket to remain empty for non-IP 127.* hostname, got %q", cfg.UnixSocket)
	}
	if cfg.Auth.AllowPlaintextSecretReturn {
		t.Fatalf("expected hardened profile settings to be applied after load")
	}
}

func TestLoadHardenedProfileWithLoopbackAddressDefaultsUnixSocket(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg-hardened-local.json")
	data := `{"security_profile":"hardened","address":"127.0.0.1:8765","unix_socket":""}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnixSocket == "" {
		t.Fatalf("expected hardened loopback config to default unix socket")
	}
}

func TestDefaultRequestPolicyConfig(t *testing.T) {
	cfg := Default()
	if cfg.RequestPolicy.IdenticalRequestCooldownSeconds != 60 {
		t.Fatalf("expected cooldown default of 60 seconds, got %d", cfg.RequestPolicy.IdenticalRequestCooldownSeconds)
	}
	if cfg.RequestPolicy.MaxPendingPerAgent != 2 {
		t.Fatalf("expected max pending per agent default of 2, got %d", cfg.RequestPolicy.MaxPendingPerAgent)
	}
	if !cfg.RequestPolicy.EnableActiveLeaseReuse {
		t.Fatalf("expected active lease reuse to default to enabled")
	}
}

func TestLoadRequestPolicyConfigOverrides(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg-request-policy.json")
	data := `{"request_policy":{"identical_request_cooldown_seconds":120,"max_pending_per_agent":4,"enable_active_lease_reuse":false}}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RequestPolicy.IdenticalRequestCooldownSeconds != 120 {
		t.Fatalf("expected cooldown override of 120 seconds, got %d", cfg.RequestPolicy.IdenticalRequestCooldownSeconds)
	}
	if cfg.RequestPolicy.MaxPendingPerAgent != 4 {
		t.Fatalf("expected max pending override of 4, got %d", cfg.RequestPolicy.MaxPendingPerAgent)
	}
	if cfg.RequestPolicy.EnableActiveLeaseReuse {
		t.Fatalf("expected active lease reuse override to be disabled")
	}
}

func TestLoadRequestPolicyConfigNormalizesInvalidNumericValues(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg-request-policy-normalize.json")
	data := `{"request_policy":{"identical_request_cooldown_seconds":0,"max_pending_per_agent":0}}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RequestPolicy.IdenticalRequestCooldownSeconds != 60 {
		t.Fatalf("expected cooldown to normalize to 60 seconds, got %d", cfg.RequestPolicy.IdenticalRequestCooldownSeconds)
	}
	if cfg.RequestPolicy.MaxPendingPerAgent != 2 {
		t.Fatalf("expected max pending to normalize to 2, got %d", cfg.RequestPolicy.MaxPendingPerAgent)
	}
}
