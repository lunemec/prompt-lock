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

func TestScanSkipsRepoLocalGoCacheDirs(t *testing.T) {
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

func TestScanDoesNotSkipNestedGoCacheDirs(t *testing.T) {
	root := t.TempDir()

	nestedGoCachePath := filepath.Join(root, "sub", ".gocache", "leak.txt")
	if err := os.MkdirAll(filepath.Dir(nestedGoCachePath), 0o755); err != nil {
		t.Fatalf("mkdir nested .gocache: %v", err)
	}
	if err := os.WriteFile(nestedGoCachePath, []byte("token ghp_nested"), 0o644); err != nil {
		t.Fatalf("write nested .gocache file: %v", err)
	}

	nestedGoModCachePath := filepath.Join(root, "sub", ".gomodcache", "pkg", "mod", "leak.txt")
	if err := os.MkdirAll(filepath.Dir(nestedGoModCachePath), 0o755); err != nil {
		t.Fatalf("mkdir nested .gomodcache: %v", err)
	}
	if err := os.WriteFile(nestedGoModCachePath, []byte("binary bytes ... AKIA ..."), 0o644); err != nil {
		t.Fatalf("write nested .gomodcache file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected nested go cache dirs to stay scanned, got %#v", violations)
	}
	if !strings.Contains(violations[0].path, "sub/.gocache/leak.txt") && !strings.Contains(violations[1].path, "sub/.gocache/leak.txt") {
		t.Fatalf("expected nested .gocache violation, got %#v", violations)
	}
	if !strings.Contains(violations[0].path, "sub/.gomodcache/pkg/mod/leak.txt") && !strings.Contains(violations[1].path, "sub/.gomodcache/pkg/mod/leak.txt") {
		t.Fatalf("expected nested .gomodcache violation, got %#v", violations)
	}
}

func TestScanSkipsRepoLocalGoBuildCacheDir(t *testing.T) {
	root := t.TempDir()
	goBuildPath := filepath.Join(root, ".cache", "go-build", "ab", "cache.bin")
	if err := os.MkdirAll(filepath.Dir(goBuildPath), 0o755); err != nil {
		t.Fatalf("mkdir .cache/go-build: %v", err)
	}
	if err := os.WriteFile(goBuildPath, []byte("binary bytes ... ghp_ ..."), 0o644); err != nil {
		t.Fatalf("write .cache/go-build file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations from repo-local go build cache dir, got %#v", violations)
	}
}

func TestScanDoesNotSkipUnrelatedDotCacheDirs(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".cache", "custom", "leak.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir .cache/custom: %v", err)
	}
	if err := os.WriteFile(target, []byte("token ghp_abc"), 0o644); err != nil {
		t.Fatalf("write .cache/custom file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected unrelated .cache dir to stay scanned, got %#v", violations)
	}
	if !strings.Contains(violations[0].path, ".cache/custom/leak.txt") {
		t.Fatalf("unexpected path: %#v", violations)
	}
}

func TestScanDoesNotSkipNestedScannerSourceDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "sub", "cmd", "promptlock-validate-security", "leak.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir nested scanner source dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("token ghp_abc"), 0o644); err != nil {
		t.Fatalf("write nested scanner source file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected nested scanner source dir to stay scanned, got %#v", violations)
	}
	if !strings.Contains(violations[0].path, "sub/cmd/promptlock-validate-security/leak.txt") {
		t.Fatalf("unexpected path: %#v", violations)
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
