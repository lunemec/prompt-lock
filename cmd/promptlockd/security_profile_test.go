package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestValidateSecurityProfile(t *testing.T) {
	mk := func(profile string, auth bool) config.Config {
		return config.Config{SecurityProfile: profile, Auth: config.AuthConfig{EnableAuth: auth}}
	}

	if err := validateSecurityProfile(mk("dev", false), ""); err != nil {
		t.Fatalf("dev profile should be allowed: %v", err)
	}
	if err := validateSecurityProfile(mk("hardened", false), ""); err == nil {
		t.Fatalf("hardened profile without auth should fail")
	}
	if err := validateSecurityProfile(mk("hardened", true), ""); err != nil {
		t.Fatalf("hardened with auth should pass: %v", err)
	}
	if err := validateSecurityProfile(mk("insecure", true), ""); err == nil {
		t.Fatalf("insecure profile without explicit opt-in should fail")
	}
	if err := validateSecurityProfile(mk("insecure", true), "1"); err != nil {
		t.Fatalf("insecure profile with explicit opt-in should pass: %v", err)
	}
}
