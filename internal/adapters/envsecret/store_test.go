package envsecret

import (
	"os"
	"testing"
)

func TestGetSecretFromEnv(t *testing.T) {
	t.Setenv("PROMPTLOCK_SECRET_GITHUB_TOKEN", "abc123")
	s := New("PROMPTLOCK_SECRET_")
	v, err := s.GetSecret("github_token")
	if err != nil {
		t.Fatalf("expected secret, got err %v", err)
	}
	if v != "abc123" {
		t.Fatalf("unexpected value %q", v)
	}
}

func TestGetSecretMissing(t *testing.T) {
	_ = os.Unsetenv("PROMPTLOCK_SECRET_MISSING")
	s := New("PROMPTLOCK_SECRET_")
	if _, err := s.GetSecret("missing"); err == nil {
		t.Fatalf("expected missing secret error")
	}
}

func TestGetSecretPreservesWhitespace(t *testing.T) {
	t.Setenv("PROMPTLOCK_SECRET_GITHUB_TOKEN", "  abc123  \n")
	s := New("PROMPTLOCK_SECRET_")
	v, err := s.GetSecret("github_token")
	if err != nil {
		t.Fatalf("expected secret, got err %v", err)
	}
	if v != "  abc123  \n" {
		t.Fatalf("unexpected value %q", v)
	}
}
