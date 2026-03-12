package envpath

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveRejectsTraversalEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside-traversal.env")
	if err := os.WriteFile(outside, []byte("github_token=abc\n"), 0o600); err != nil {
		t.Fatalf("write outside env: %v", err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	_, _, err = store.Resolve("../outside-traversal.env", []string{"github_token"})
	if err == nil {
		t.Fatalf("expected traversal escape rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed root") {
		t.Fatalf("expected outside-root error, got %v", err)
	}
}

func TestResolveRejectsAbsoluteEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside-absolute.env")
	if err := os.WriteFile(outside, []byte("github_token=abc\n"), 0o600); err != nil {
		t.Fatalf("write outside env: %v", err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	_, _, err = store.Resolve(outside, []string{"github_token"})
	if err == nil {
		t.Fatalf("expected absolute escape rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed root") {
		t.Fatalf("expected outside-root error, got %v", err)
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside-symlink.env")
	if err := os.WriteFile(outside, []byte("github_token=abc\n"), 0o600); err != nil {
		t.Fatalf("write outside env: %v", err)
	}
	linkPath := filepath.Join(root, "link.env")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	_, _, err = store.Resolve("link.env", []string{"github_token"})
	if err == nil {
		t.Fatalf("expected symlink escape rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed root") {
		t.Fatalf("expected outside-root error, got %v", err)
	}
}

func TestResolveReturnsOnlyRequestedKeysWithinRoot(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "secrets")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir env dir: %v", err)
	}
	envFile := filepath.Join(envDir, ".env")
	payload := strings.Join([]string{
		"# comment",
		"github_token=abc",
		"npm_token='def'",
		"unused_token=xyz",
		"",
	}, "\n")
	if err := os.WriteFile(envFile, []byte(payload), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	resolved, canonicalPath, err := store.Resolve("secrets/.env", []string{"github_token", "npm_token"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if canonicalPath != envFile {
		t.Fatalf("expected canonical path %q, got %q", envFile, canonicalPath)
	}

	expected := map[string]string{
		"github_token": "abc",
		"npm_token":    "def",
	}
	if !reflect.DeepEqual(expected, resolved) {
		t.Fatalf("unexpected resolved map: got=%v want=%v", resolved, expected)
	}
	if _, ok := resolved["unused_token"]; ok {
		t.Fatalf("unexpected unrequested key in resolved map")
	}
}
