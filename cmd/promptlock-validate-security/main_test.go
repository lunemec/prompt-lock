package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanDetectsForbiddenPattern(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "x.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("token ghp_abc"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if !strings.Contains(violations[0].path, "docs/x.txt") {
		t.Fatalf("unexpected path: %q", violations[0].path)
	}
}

func TestScanSkipsSelfAndPyc(t *testing.T) {
	root := t.TempDir()
	selfPath := filepath.Join(root, "cmd", "promptlock-validate-security", "main.go")
	if err := os.MkdirAll(filepath.Dir(selfPath), 0o755); err != nil {
		t.Fatalf("mkdir self: %v", err)
	}
	if err := os.WriteFile(selfPath, []byte("ghp_"), 0o644); err != nil {
		t.Fatalf("write self: %v", err)
	}

	pycPath := filepath.Join(root, "a.pyc")
	if err := os.WriteFile(pycPath, []byte("ghp_"), 0o644); err != nil {
		t.Fatalf("write pyc: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
