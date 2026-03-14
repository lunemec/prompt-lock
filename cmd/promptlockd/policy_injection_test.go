package main

import (
	"fmt"
	"testing"

	"github.com/lunemec/promptlock/internal/app"
)

type mockPolicy struct{}

func (mockPolicy) ValidateExecuteRequest(string, app.ExecuteRequest) error {
	return fmt.Errorf("blocked-by-mock")
}
func (mockPolicy) ValidateExecuteCommand([]string) error { return nil }
func (mockPolicy) ResolveExecuteCommand(cmd []string) (app.ResolvedCommand, error) {
	return app.ResolvedCommand{Path: "go", Args: append([]string{}, cmd[1:]...), SearchPath: "/usr/bin"}, nil
}
func (mockPolicy) ValidateNetworkEgress([]string, string) error { return nil }
func (mockPolicy) ValidateHostDockerCommand([]string) error     { return nil }
func (mockPolicy) ResolveHostDockerCommand(cmd []string) (app.ResolvedCommand, error) {
	return app.ResolvedCommand{Path: "docker", Args: append([]string{}, cmd[1:]...), SearchPath: "/usr/bin"}, nil
}
func (mockPolicy) ApplyOutputSecurity(in string) string { return in }
func (mockPolicy) ClampTimeout(requested int) int       { return requested }

func TestControlPolicyCanBeInjected(t *testing.T) {
	s := &server{policyEngine: mockPolicy{}}
	err := s.validateExecuteRequest(executeReq{Intent: "x", Command: []string{"go", "version"}})
	if err == nil || err.Error() != "blocked-by-mock" {
		t.Fatalf("expected injected policy denial, got %v", err)
	}
}
