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
	data := `{"address":":9999","policy":{"default_ttl_minutes":7,"min_ttl_minutes":1,"max_ttl_minutes":20,"max_secrets_per_request":3}}`
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
}
