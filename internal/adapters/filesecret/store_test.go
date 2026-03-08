package filesecret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreLoadAndGet(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(p, []byte(`{"github_token":"abc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(p)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	v, err := s.GetSecret("github_token")
	if err != nil || v != "abc" {
		t.Fatalf("expected abc, got %q err=%v", v, err)
	}
}

func TestFileStoreMissingSecret(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(p, []byte(`{"x":"y"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(p)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := s.GetSecret("missing"); err == nil {
		t.Fatalf("expected missing secret error")
	}
}
