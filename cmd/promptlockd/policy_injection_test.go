package main

import (
	"fmt"
	"testing"

	"github.com/lunemec/promptlock/internal/app"
)

type mockPolicy struct{}

func (mockPolicy) ValidateExecuteRequest(string, app.ExecuteRequest) error { return fmt.Errorf("blocked-by-mock") }
func (mockPolicy) ValidateExecuteCommand([]string) error                    { return nil }
func (mockPolicy) ValidateNetworkEgress([]string, string) error             { return nil }
func (mockPolicy) ValidateHostDockerCommand([]string) error                 { return nil }
func (mockPolicy) ApplyOutputSecurity(in string) string                     { return in }
func (mockPolicy) ClampTimeout(requested int) int                           { return requested }

func TestControlPolicyCanBeInjected(t *testing.T) {
	s := &server{policyEngine: mockPolicy{}}
	err := s.validateExecuteRequest(executeReq{Intent: "x", Command: []string{"go", "version"}})
	if err == nil || err.Error() != "blocked-by-mock" {
		t.Fatalf("expected injected policy denial, got %v", err)
	}
}
