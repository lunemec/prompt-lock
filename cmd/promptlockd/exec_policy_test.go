package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestValidateExecuteCommand(t *testing.T) {
	s := &server{execPolicy: config.ExecutionPolicy{AllowlistPrefixes: []string{"bash", "go"}, DenylistSubstrings: []string{"printenv"}}}
	if err := s.validateExecuteCommand([]string{"bash", "-lc", "echo ok"}); err != nil {
		t.Fatalf("expected allowed command: %v", err)
	}
	if err := s.validateExecuteCommand([]string{"python", "-c", "print(1)"}); err == nil {
		t.Fatalf("expected allowlist rejection")
	}
	if err := s.validateExecuteCommand([]string{"bash", "-lc", "printenv"}); err == nil {
		t.Fatalf("expected denylist rejection")
	}
}

func TestApplyOutputSecurity(t *testing.T) {
	in := "secret=abc"
	if got := applyOutputSecurity("none", in); got != "" {
		t.Fatalf("expected none mode to suppress output")
	}
	if got := applyOutputSecurity("raw", in); got != in {
		t.Fatalf("expected raw mode passthrough")
	}
	if got := applyOutputSecurity("redacted", in); got == in {
		t.Fatalf("expected redacted mode to modify sensitive markers")
	}
}
