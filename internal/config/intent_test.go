package config

import "testing"

func TestResolveIntent(t *testing.T) {
	cfg := Default()
	cfg.Intents = map[string][]string{
		"run_tests": {"github_token"},
	}

	s, err := cfg.ResolveIntent("run_tests")
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0] != "github_token" {
		t.Fatalf("unexpected secrets: %#v", s)
	}

	if _, err := cfg.ResolveIntent("missing"); err == nil {
		t.Fatalf("expected unknown intent error")
	}
}
