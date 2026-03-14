package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestValidateExecuteCommand(t *testing.T) {
	s := &server{execPolicy: config.ExecutionPolicy{ExactMatchExecutables: []string{"bash", "go"}, DenylistSubstrings: []string{"printenv"}}}
	if err := s.validateExecuteCommand([]string{"bash", "-lc", "echo ok"}); err != nil {
		t.Fatalf("expected allowed command: %v", err)
	}
	if err := s.validateExecuteCommand([]string{"python", "-c", "print(1)"}); err == nil {
		t.Fatalf("expected allowlist rejection")
	}
	if err := s.validateExecuteCommand([]string{"bash", "-lc", "printenv"}); err == nil {
		t.Fatalf("expected denylist rejection")
	}
	sSmuggle := &server{execPolicy: config.ExecutionPolicy{ExactMatchExecutables: []string{"go"}, DenylistSubstrings: []string{"&&"}}}
	if err := sSmuggle.validateExecuteCommand([]string{"go", "test", "&&", "curl"}); err == nil {
		t.Fatalf("expected smuggling token rejection")
	}
	if err := sSmuggle.validateExecuteCommand([]string{"goevil", "test"}); err == nil {
		t.Fatalf("expected exact executable identity rejection for goevil")
	}
	if err := sSmuggle.validateExecuteCommand([]string{"git-backdoor", "status"}); err == nil {
		t.Fatalf("expected exact executable identity rejection for git-backdoor")
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
