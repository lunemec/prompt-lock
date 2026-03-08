package main

import (
	"strings"
	"testing"
)

func TestWithPolicyHintAddsActionableHint(t *testing.T) {
	got := withPolicyHint("docker compose verb \"up\" not allowed")
	if !strings.Contains(got, "hint:") {
		t.Fatalf("expected hint in message, got %q", got)
	}
}

func TestWithPolicyHintNoHintForUnknown(t *testing.T) {
	in := "some unrelated error"
	got := withPolicyHint(in)
	if got != in {
		t.Fatalf("expected unchanged message, got %q", got)
	}
}
