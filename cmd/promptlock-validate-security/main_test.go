package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func githubToken() string { return "ghp_" + strings.Repeat("a", 20) }

func nestedGitHubToken() string { return "ghp_nested" + strings.Repeat("b", 20) }

func githubFineGrainedToken() string { return "github_pat_" + strings.Repeat("c", 20) }

func awsAccessKey() string { return "AKIA" + strings.Repeat("E", 16) }

func bearerToken() string { return "Bearer " + strings.Repeat("g", 20) }

func auditBearerToken() string { return "Bearer sess_" + strings.Repeat("b", 32) }

func openAILiveKey() string { return "sk-live-" + strings.Repeat("d", 12) }

func openAIGenericKey() string { return "sk-" + strings.Repeat("f", 20) }

func historicalSecurityBasicsScript() string {
	return strings.Join([]string{
		"#!/usr/bin/env python3",
		"from pathlib import Path",
		"import sys",
		"",
		"# Basic policy guardrails for prototype repo",
		"forbidden = [",
		"    'ghp_',",
		"    'sk-live-',",
		"    'AKIA',",
		"    '-----BEGIN PRIVATE KEY-----',",
		"]",
		"",
		"violations = []",
		"for path in Path('.').rglob('*'):",
		"    if not path.is_file():",
		"        continue",
		"    if '.git/' in str(path):",
		"        continue",
		"    if str(path).endswith('scripts/validate_security_basics.py'):",
		"        continue",
		"    if path.suffix in {'.pyc'}:",
		"        continue",
		"    try:",
		"        txt = path.read_text(encoding='utf-8', errors='ignore')",
		"    except Exception:",
		"        continue",
		"    for token in forbidden:",
		"        if token in txt:",
		"            violations.append((str(path), token))",
		"",
		"if violations:",
		"    print('Security baseline failed: possible secret patterns found:')",
		"    for v in violations:",
		"        print(f' - {v[0]} contains pattern {v[1]!r}')",
		"    sys.exit(1)",
		"",
		"print('Security baseline checks passed')",
		"",
	}, "\n")
}

func hasViolation(violations []violation, path, pattern string) bool {
	for _, v := range violations {
		if v.path == path && v.pattern == pattern {
			return true
		}
	}
	return false
}

func TestScanDetectsForbiddenPattern(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "x.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("token "+githubToken()), 0o644); err != nil {
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

func TestScanDetectsModernTokenPattern(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "modern.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte(githubFineGrainedToken()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].pattern != "GitHub fine-grained token" {
		t.Fatalf("unexpected pattern: %q", violations[0].pattern)
	}
}

func TestScanDetectsBearerTokenPattern(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "bearer.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("Authorization: "+bearerToken()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].pattern != "Bearer token" {
		t.Fatalf("unexpected pattern: %q", violations[0].pattern)
	}
}

func TestScanDetectsOpenAIKeyPattern(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "openai.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte(openAIGenericKey()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	violations, err := scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].pattern != "OpenAI API key" {
		t.Fatalf("unexpected pattern: %q", violations[0].pattern)
	}
}

func TestScanSkipsSelfDirAndPyc(t *testing.T) {
	root := t.TempDir()
	selfPath := filepath.Join(root, "cmd", "promptlock-validate-security", "main.go")
	if err := os.MkdirAll(filepath.Dir(selfPath), 0o755); err != nil {
		t.Fatalf("mkdir self: %v", err)
	}
	if err := os.WriteFile(selfPath, []byte(githubToken()), 0o644); err != nil {
		t.Fatalf("write self: %v", err)
	}

	pycPath := filepath.Join(root, "a.pyc")
	if err := os.WriteFile(pycPath, []byte(githubToken()), 0o644); err != nil {
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

func TestScanAllowsExactFixtureLiteralsInKnownFixtureFiles(t *testing.T) {
	cases := []struct {
		name string
		path string
		data string
	}{
		{
			name: "audit fixture file",
			path: "internal/adapters/audit/file_test.go",
			data: "note: " + auditBearerToken(),
		},
		{
			name: "execution env test file",
			path: "internal/app/execution_process_test.go",
			data: "Authorization: Bearer super-secret-bearer-token",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(target, []byte(tc.data), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			violations, err := scan(root)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(violations) != 0 {
				t.Fatalf("expected exact fixture literal to stay allowed, got %#v", violations)
			}
		})
	}
}

func TestScanFlagsSecretsInFormerlyAllowedFixtureFiles(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		content     string
		wantPattern []string
	}{
		{
			name: "demo env file",
			path: "demo-envs/github.env",
			content: strings.Join([]string{
				"github_token=" + githubToken(),
				"OPENAI_API_KEY=" + openAILiveKey(),
			}, "\n"),
			wantPattern: []string{"GitHub token", "OpenAI live key"},
		},
		{
			name: "deleted python policy script",
			path: "scripts/validate_security_basics.py",
			content: strings.Join([]string{
				"-----BEGIN PRIVATE KEY-----",
				"token " + githubToken(),
			}, "\n"),
			wantPattern: []string{"Private key", "GitHub token"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(target, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			violations, err := scan(root)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(violations) != len(tc.wantPattern) {
				t.Fatalf("expected %d violations, got %#v", len(tc.wantPattern), violations)
			}
			for _, pattern := range tc.wantPattern {
				if !hasViolation(violations, tc.path, pattern) {
					t.Fatalf("expected %q violation at %s, got %#v", pattern, tc.path, violations)
				}
			}
		})
	}
}

func TestScanGitHistorySkipsKnownHistoricalFixtureBlob(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git test setup differs on windows")
	}

	root := t.TempDir()
	mustGit(t, root, "init")
	mustGit(t, root, "config", "user.name", "PromptLock Test")
	mustGit(t, root, "config", "user.email", "promptlock@example.invalid")

	secretPath := filepath.Join(root, "scripts", "validate_security_basics.py")
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o755); err != nil {
		t.Fatalf("mkdir history fixture: %v", err)
	}
	if err := os.WriteFile(secretPath, []byte(historicalSecurityBasicsScript()), 0o755); err != nil {
		t.Fatalf("write history fixture: %v", err)
	}
	mustGit(t, root, "add", "scripts/validate_security_basics.py")
	mustGit(t, root, "commit", "-m", "add historical security fixture")
	if err := os.Remove(secretPath); err != nil {
		t.Fatalf("remove history fixture: %v", err)
	}
	mustGit(t, root, "commit", "-am", "remove historical security fixture")

	violations, err := scanGitHistory(root)
	if err != nil {
		t.Fatalf("scan history: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected known historical fixture blob to be allowlisted, got %#v", violations)
	}
}

func TestScanSkipsReleaseArtifactDirs(t *testing.T) {
	root := t.TempDir()
	distPath := filepath.Join(root, "dist", "promptlock-0.2.0.tar.gz")
	if err := os.MkdirAll(filepath.Dir(distPath), 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(distPath, []byte("binary bytes ... "+awsAccessKey()+" ..."), 0o644); err != nil {
		t.Fatalf("write dist: %v", err)
	}

	goreleaserPath := filepath.Join(root, ".goreleaser-dist", "promptlock-linux-amd64")
	if err := os.MkdirAll(filepath.Dir(goreleaserPath), 0o755); err != nil {
		t.Fatalf("mkdir goreleaser: %v", err)
	}
	if err := os.WriteFile(goreleaserPath, []byte("binary bytes ... "+awsAccessKey()+" ..."), 0o644); err != nil {
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
	if err := os.WriteFile(goCachePath, []byte("binary bytes ... "+githubToken()+" ..."), 0o644); err != nil {
		t.Fatalf("write .gocache file: %v", err)
	}

	goModCachePath := filepath.Join(root, ".gomodcache", "pkg", "mod", "cache.bin")
	if err := os.MkdirAll(filepath.Dir(goModCachePath), 0o755); err != nil {
		t.Fatalf("mkdir .gomodcache: %v", err)
	}
	if err := os.WriteFile(goModCachePath, []byte("binary bytes ... "+awsAccessKey()+" ..."), 0o644); err != nil {
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
	if err := os.WriteFile(nestedGoCachePath, []byte("token "+nestedGitHubToken()), 0o644); err != nil {
		t.Fatalf("write nested .gocache file: %v", err)
	}

	nestedGoModCachePath := filepath.Join(root, "sub", ".gomodcache", "pkg", "mod", "leak.txt")
	if err := os.MkdirAll(filepath.Dir(nestedGoModCachePath), 0o755); err != nil {
		t.Fatalf("mkdir nested .gomodcache: %v", err)
	}
	if err := os.WriteFile(nestedGoModCachePath, []byte("binary bytes ... "+awsAccessKey()+" ..."), 0o644); err != nil {
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
	if err := os.WriteFile(goBuildPath, []byte("binary bytes ... "+githubToken()+" ..."), 0o644); err != nil {
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
	if err := os.WriteFile(target, []byte("token "+githubToken()), 0o644); err != nil {
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
	if err := os.WriteFile(target, []byte("token "+githubToken()), 0o644); err != nil {
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

func TestScanGitHistoryDetectsDeletedSecretBlob(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git test setup differs on windows")
	}

	root := t.TempDir()
	mustGit(t, root, "init")
	mustGit(t, root, "config", "user.name", "PromptLock Test")
	mustGit(t, root, "config", "user.email", "promptlock@example.invalid")

	secretPath := filepath.Join(root, "docs", "history.txt")
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o755); err != nil {
		t.Fatalf("mkdir history fixture: %v", err)
	}
	if err := os.WriteFile(secretPath, []byte("token "+githubToken()), 0o644); err != nil {
		t.Fatalf("write secret fixture: %v", err)
	}
	mustGit(t, root, "add", "docs/history.txt")
	mustGit(t, root, "commit", "-m", "add secret")
	if err := os.WriteFile(secretPath, []byte("safe content"), 0o644); err != nil {
		t.Fatalf("rewrite secret fixture: %v", err)
	}
	mustGit(t, root, "commit", "-am", "remove secret")

	violations, err := scanGitHistory(root)
	if err != nil {
		t.Fatalf("scan history: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 historical violation, got %#v", violations)
	}
	if !strings.Contains(violations[0].path, "docs/history.txt") {
		t.Fatalf("unexpected history violation path: %#v", violations)
	}
}

func mustGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, strings.TrimSpace(string(out)))
	}
}
