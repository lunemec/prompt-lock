package main

import (
	"strings"
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestNetworkEgressPolicy(t *testing.T) {
	s := &server{networkEgressPolicy: config.NetworkEgressPolicy{
		Enabled:        true,
		AllowDomains:   []string{"example.com", "api.github.com"},
		DenySubstrings: []string{"169.254.169.254"},
	}}
	if err := s.validateNetworkEgress([]string{"curl", "https://api.github.com/repos"}, ""); err != nil {
		t.Fatalf("expected allowed domain: %v", err)
	}
	if err := s.validateNetworkEgress([]string{"curl", "https://evil.com"}, ""); err == nil {
		t.Fatalf("expected blocked domain")
	}
	if err := s.validateNetworkEgress([]string{"curl", "http://169.254.169.254/latest/meta-data"}, ""); err == nil {
		t.Fatalf("expected deny substring block")
	}
}

func TestNetworkEgressExtractsNonURLDomainForms(t *testing.T) {
	s := &server{networkEgressPolicy: config.NetworkEgressPolicy{
		Enabled:      true,
		AllowDomains: []string{"api.github.com"},
	}}
	if err := s.validateNetworkEgress([]string{"curl", "api.github.com"}, ""); err != nil {
		t.Fatalf("expected bare domain to be allowed: %v", err)
	}
	if err := s.validateNetworkEgress([]string{"curl", "--host", "api.github.com"}, ""); err != nil {
		t.Fatalf("expected --host form to be allowed: %v", err)
	}
	if err := s.validateNetworkEgress([]string{"curl", "--url", "https://api.github.com/repos"}, ""); err != nil {
		t.Fatalf("expected --url form to be allowed: %v", err)
	}
	if err := s.validateNetworkEgress([]string{"curl", "api.github.com/repos"}, ""); err != nil {
		t.Fatalf("expected host/path form to be allowed: %v", err)
	}
	if err := s.validateNetworkEgress([]string{"curl", "api.github.com:443/repos"}, ""); err != nil {
		t.Fatalf("expected host:port/path form to be allowed: %v", err)
	}
}

func TestNetworkEgressBlocksHostPathBypass(t *testing.T) {
	s := &server{networkEgressPolicy: config.NetworkEgressPolicy{
		Enabled:      true,
		AllowDomains: []string{"api.github.com"},
	}}
	if err := s.validateNetworkEgress([]string{"curl", "evil.com/path"}, ""); err == nil {
		t.Fatalf("expected host/path bypass form to be blocked")
	}
	if err := s.validateNetworkEgress([]string{"wget", "evil.com/file"}, ""); err == nil {
		t.Fatalf("expected alternate client host/path form to be blocked")
	}
}

func TestNetworkEgressIntentDeterministic(t *testing.T) {
	s := &server{networkEgressPolicy: config.NetworkEgressPolicy{
		Enabled:            true,
		RequireIntentMatch: true,
		AllowDomains:       []string{"fallback.example.com"},
		IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
	}}
	if err := s.validateNetworkEgress([]string{"curl", "https://api.github.com/repos"}, "run_tests"); err != nil {
		t.Fatalf("expected intent allow to pass: %v", err)
	}
	if err := s.validateNetworkEgress([]string{"curl", "https://fallback.example.com"}, "run_tests"); err == nil {
		t.Fatalf("expected fallback domain blocked when intent map exists")
	}
	if err := s.validateNetworkEgress([]string{"curl", "https://api.github.com/repos"}, ""); err == nil {
		t.Fatalf("expected missing intent to fail when require_intent_match=true")
	}
}

func TestNetworkEgressBlocksPrivateIPTargets(t *testing.T) {
	s := &server{networkEgressPolicy: config.NetworkEgressPolicy{
		Enabled:      true,
		AllowDomains: []string{"10.0.0.1", "127.0.0.1"},
	}}
	if err := s.validateNetworkEgress([]string{"curl", "http://127.0.0.1:8080"}, ""); err == nil {
		t.Fatalf("expected loopback IP to be blocked")
	}
	if err := s.validateNetworkEgress([]string{"curl", "10.0.0.1"}, ""); err == nil {
		t.Fatalf("expected private IP to be blocked")
	}
}

func TestNetworkEgressRejectsDirectNetworkClientWithoutInspectableDestination(t *testing.T) {
	s := &server{networkEgressPolicy: config.NetworkEgressPolicy{
		Enabled:            true,
		RequireIntentMatch: true,
		IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
	}}
	err := s.validateNetworkEgress([]string{"curl", "--config", "./agent-controlled.cfg"}, "run_tests")
	if err == nil {
		t.Fatalf("expected direct network client without inspectable destination to be blocked")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "inspectable destination") {
		t.Fatalf("expected inspectable-destination deny detail, got %v", err)
	}
}
