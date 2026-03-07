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
