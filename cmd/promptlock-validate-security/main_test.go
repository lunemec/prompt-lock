package main

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestScanSkipsReleaseArtifactDirs(t *testing.T) {
	root := t.TempDir()
	distPath := filepath.Join(root, "dist", "promptlock-0.2.0.tar.gz")
	if err := os.MkdirAll(filepath.Dir(distPath), 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(distPath, []byte("binary bytes ... AKIA ..."), 0o644); err != nil {
		t.Fatalf("write dist: %v", err)
	}

	goreleaserPath := filepath.Join(root, ".goreleaser-dist", "promptlock-linux-amd64")
	if err := os.MkdirAll(filepath.Dir(goreleaserPath), 0o755); err != nil {
		t.Fatalf("mkdir goreleaser: %v", err)
	}
	if err := os.WriteFile(goreleaserPath, []byte("binary bytes ... AKIA ..."), 0o644); err != nil {
		t.Fatalf("write goreleaser: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations from release artifact dirs, got %#v", violations)
	}
}

func TestScanSkipsGoCacheDirs(t *testing.T) {
	root := t.TempDir()

	goCachePath := filepath.Join(root, ".gocache", "binary")
	if err := os.MkdirAll(filepath.Dir(goCachePath), 0o755); err != nil {
		t.Fatalf("mkdir .gocache: %v", err)
	}
	if err := os.WriteFile(goCachePath, []byte("binary bytes ... ghp_ ..."), 0o644); err != nil {
		t.Fatalf("write .gocache file: %v", err)
	}

	goModCachePath := filepath.Join(root, ".gomodcache", "pkg", "mod", "cache.bin")
	if err := os.MkdirAll(filepath.Dir(goModCachePath), 0o755); err != nil {
		t.Fatalf("mkdir .gomodcache: %v", err)
	}
	if err := os.WriteFile(goModCachePath, []byte("binary bytes ... AKIA ..."), 0o644); err != nil {
		t.Fatalf("write .gomodcache file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations from go cache dirs, got %#v", violations)
	}
}

func TestScanFailsClosedOnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-denied semantics differ on windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "restricted.txt")
	if err := os.WriteFile(target, []byte("safe-content"), 0o600); err != nil {
		t.Fatalf("write restricted file: %v", err)
	}
	if err := os.Chmod(target, 0); err != nil {
		t.Fatalf("chmod restricted file: %v", err)
	}

	_, err := scan(root)
	if err == nil {
		t.Fatalf("expected unreadable file to fail the scan")
	}
	if !strings.Contains(err.Error(), "restricted.txt") {
		t.Fatalf("expected error to mention unreadable path, got %v", err)
	}
}

func TestScanFailsClosedOnUnreadableSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink handling differs on windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "missing.txt")
	link := filepath.Join(root, "broken-link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := scan(root)
	if err == nil {
		t.Fatalf("expected broken symlink to fail the scan")
	}
	if !strings.Contains(err.Error(), "broken-link.txt") {
		t.Fatalf("expected error to mention unreadable path, got %v", err)
	}
}
