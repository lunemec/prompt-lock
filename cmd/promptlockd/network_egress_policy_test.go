package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestNetworkEgressPolicy(t *testing.T) {
	s := &server{networkEgressPolicy: config.NetworkEgressPolicy{
		Enabled:        true,
		AllowDomains:   []string{"example.com", "api.github.com"},
		DenySubstrings: []string{"169.254.169.254"},
	}}
	if err := s.validateNetworkEgress([]string{"bash", "-lc", "curl https://api.github.com/repos"}); err != nil {
		t.Fatalf("expected allowed domain: %v", err)
	}
	if err := s.validateNetworkEgress([]string{"bash", "-lc", "curl https://evil.com"}); err == nil {
		t.Fatalf("expected blocked domain")
	}
	if err := s.validateNetworkEgress([]string{"bash", "-lc", "curl http://169.254.169.254/latest/meta-data"}); err == nil {
		t.Fatalf("expected deny substring block")
	}
}
