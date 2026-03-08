package main

import (
	"strings"
	"testing"
)

func TestNewSecureTokenFormatAndLength(t *testing.T) {
	tok, err := newSecureToken("lease_")
	if err != nil {
		t.Fatalf("newSecureToken failed: %v", err)
	}
	if !strings.HasPrefix(tok, "lease_") {
		t.Fatalf("token missing prefix: %q", tok)
	}
	// 32 random bytes encoded as hex -> 64 chars, plus prefix length 6
	if got, want := len(tok), len("lease_")+64; got != want {
		t.Fatalf("unexpected token length: got %d want %d", got, want)
	}
}

func TestNewSecureTokenUniqueness(t *testing.T) {
	const n = 5000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		tok, err := newSecureToken("sess_")
		if err != nil {
			t.Fatalf("newSecureToken failed at %d: %v", i, err)
		}
		if _, exists := seen[tok]; exists {
			t.Fatalf("duplicate token generated at iteration %d", i)
		}
		seen[tok] = struct{}{}
	}
}
