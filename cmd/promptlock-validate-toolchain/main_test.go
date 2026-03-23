package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateToolchainFilesPasses(t *testing.T) {
	cfg := sampleToolchainConfig()
	if err := validateToolchainFiles(cfg, sampleToolchainFiles(cfg)); err != nil {
		t.Fatalf("expected validation to pass, got %v", err)
	}
}

func TestRunValidatesModuleReproducibility(t *testing.T) {
	root := t.TempDir()
	cfg := sampleToolchainConfig()
	writeToolchainFixtures(t, root, cfg)
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("github.com/example/dependency v1.2.3 h1:abc123\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	var calls []string
	oldRunGoCommand := runGoCommand
	oldRunGitCommand := runGitCommand
	t.Cleanup(func() {
		runGoCommand = oldRunGoCommand
		runGitCommand = oldRunGitCommand
	})
	runGoCommand = func(actualRoot string, args ...string) ([]byte, error) {
		if actualRoot != root {
			t.Fatalf("unexpected root: got %q want %q", actualRoot, root)
		}
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	runGitCommand = func(actualRoot string, args ...string) ([]byte, error) {
		if actualRoot != root {
			t.Fatalf("unexpected root: got %q want %q", actualRoot, root)
		}
		if strings.Join(args, " ") != "ls-files --error-unmatch go.sum" {
			t.Fatalf("unexpected git command: %v", args)
		}
		return nil, nil
	}

	if err := run(root); err != nil {
		t.Fatalf("expected run to pass, got %v", err)
	}
	wantCalls := []string{"mod verify", "mod tidy -diff"}
	if len(calls) != len(wantCalls) {
		t.Fatalf("unexpected go command calls: got %#v want %#v", calls, wantCalls)
	}
	for i := range wantCalls {
		if calls[i] != wantCalls[i] {
			t.Fatalf("unexpected go command call %d: got %q want %q", i, calls[i], wantCalls[i])
		}
	}
}

func TestValidateToolchainFilesFailsWhenGoModDrifts(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["go.mod"] = "module github.com/example/project\n\ngo 1.25.0\ntoolchain go1.26.1\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected go.mod drift to fail")
	}
}

func TestValidateToolchainFilesFailsWhenWorkflowDoesNotLoadToolchainEnv(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files[".github/workflows/ci.yml"] = "name: ci\njobs:\n  validate:\n    steps:\n      - uses: actions/setup-go@v5\n        with:\n          go-version: 1.26.1\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected workflow drift to fail")
	}
}

func TestValidateToolchainFilesFailsWhenDocsLoseGuardrailReference(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["docs/operations/RELEASE.md"] = "# Release\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected release docs drift to fail")
	}
}

func TestValidateToolchainFilesFailsWhenReleasePackageDropsLauncher(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["scripts/release-package.sh"] = `#!/usr/bin/env bash
cp LICENSE README.md "$OUT_DIR/"
`

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected release packaging drift to fail")
	}
}

func TestValidateToolchainFilesFailsWhenReleasePackageAllowsGoReleaserEnvOverride(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["scripts/release-package.sh"] = "require_clean_worktree\nclean git checkout; refusing to build from a dirty tree\nGORELEASER_VERSION=\"${GORELEASER_VERSION:-v2.7.0}\"\ngo run github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION} build --clean --config .goreleaser.yaml\nLICENSE\npromptlock-mcp-launch\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected env-overridable goreleaser pin to fail")
	}
}

func TestValidateToolchainFilesFailsWhenReleasePackageUsesSnapshotBuild(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["scripts/release-package.sh"] = "require_clean_worktree\nclean git checkout; refusing to build from a dirty tree\ngo run github.com/goreleaser/goreleaser/v2@v2.7.0 build --snapshot --clean --config .goreleaser.yaml\nLICENSE\npromptlock-mcp-launch\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected snapshot release packaging to fail")
	}
}

func TestValidateToolchainFilesFailsWhenSmokeScriptReferencesPython(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["scripts/run_real_e2e_smoke.sh"] = "#!/usr/bin/env bash\npython3 - <<'PY'\nprint('nope')\nPY\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected smoke script python reference to fail")
	}
}

func TestValidateToolchainFilesFailsWhenReleaseWorkflowAlwaysPublishesPrerelease(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files[".github/workflows/release.yml"] = "permissions:\n  contents: write\nsteps:\n  - uses: softprops/action-gh-release@deadbeef\n    with:\n      prerelease: ${{ startsWith(github.ref_name, 'v0.') || contains(github.ref_name, '-rc') }}\n      files: |\n        dist/*.sha256\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected release workflow drift to fail")
	}
}

func TestValidateToolchainFilesFailsWhenWrapperDocsLoseHostAliasRestriction(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["docs/operations/WRAPPER-EXEC.md"] = "## promptlock exec\n\n- `--docker-arg` as a narrow escape hatch.\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected wrapper docs drift to fail")
	}
}

func TestValidateToolchainFilesFailsWhenReadmeLosesReleasePackagingNote(t *testing.T) {
	cfg := sampleToolchainConfig()
	files := sampleToolchainFiles(cfg)
	files["README.md"] = "## Developer And Release Workflows\n\n- Local release packaging helper: `scripts/release-package.sh <version>`\n"

	if err := validateToolchainFiles(cfg, files); err == nil {
		t.Fatalf("expected README drift to fail")
	}
}

func TestRunFailsWhenGoSumIsMissing(t *testing.T) {
	root := t.TempDir()
	cfg := sampleToolchainConfig()
	writeToolchainFixtures(t, root, cfg)

	oldRunGoCommand := runGoCommand
	oldRunGitCommand := runGitCommand
	t.Cleanup(func() {
		runGoCommand = oldRunGoCommand
		runGitCommand = oldRunGitCommand
	})
	runGoCommand = func(root string, args ...string) ([]byte, error) {
		t.Fatalf("unexpected go command when go.sum is missing: %v", args)
		return nil, nil
	}
	runGitCommand = func(root string, args ...string) ([]byte, error) {
		t.Fatalf("unexpected git command when go.sum is missing: %v", args)
		return nil, nil
	}

	err := run(root)
	if err == nil {
		t.Fatalf("expected missing go.sum to fail")
	}
	if !strings.Contains(err.Error(), "go.sum") {
		t.Fatalf("expected error to mention go.sum, got %v", err)
	}
}

func TestRunFailsWhenGoSumIsUntracked(t *testing.T) {
	root := t.TempDir()
	cfg := sampleToolchainConfig()
	writeToolchainFixtures(t, root, cfg)
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("github.com/example/dependency v1.2.3 h1:abc123\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	oldRunGoCommand := runGoCommand
	oldRunGitCommand := runGitCommand
	t.Cleanup(func() {
		runGoCommand = oldRunGoCommand
		runGitCommand = oldRunGitCommand
	})
	runGoCommand = func(root string, args ...string) ([]byte, error) {
		return nil, nil
	}
	runGitCommand = func(root string, args ...string) ([]byte, error) {
		return []byte("go.sum"), errors.New("untracked")
	}

	err := run(root)
	if err == nil {
		t.Fatalf("expected untracked go.sum to fail")
	}
	if !strings.Contains(err.Error(), "go.sum must be tracked in git") {
		t.Fatalf("expected error to mention tracked go.sum, got %v", err)
	}
}

func TestRunFailsWhenGoModVerifyDrifts(t *testing.T) {
	root := t.TempDir()
	cfg := sampleToolchainConfig()
	writeToolchainFixtures(t, root, cfg)
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("github.com/example/dependency v1.2.3 h1:abc123\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	oldRunGoCommand := runGoCommand
	oldRunGitCommand := runGitCommand
	t.Cleanup(func() {
		runGoCommand = oldRunGoCommand
		runGitCommand = oldRunGitCommand
	})
	runGoCommand = func(root string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "mod verify" {
			return []byte("module cache mismatch"), errors.New("verify drift")
		}
		t.Fatalf("unexpected go command after verify failure: %v", args)
		return nil, nil
	}
	runGitCommand = func(actualRoot string, args ...string) ([]byte, error) {
		if actualRoot != root {
			t.Fatalf("unexpected root: got %q want %q", actualRoot, root)
		}
		if strings.Join(args, " ") != "ls-files --error-unmatch go.sum" {
			t.Fatalf("unexpected git command: %v", args)
		}
		return nil, nil
	}

	err := run(root)
	if err == nil {
		t.Fatalf("expected go mod verify drift to fail")
	}
	if !strings.Contains(err.Error(), "go mod verify") {
		t.Fatalf("expected error to mention go mod verify, got %v", err)
	}
}

func TestRunFailsWhenGoModTidyDiffDrifts(t *testing.T) {
	root := t.TempDir()
	cfg := sampleToolchainConfig()
	writeToolchainFixtures(t, root, cfg)
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("github.com/example/dependency v1.2.3 h1:abc123\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	var calls []string
	oldRunGoCommand := runGoCommand
	oldRunGitCommand := runGitCommand
	t.Cleanup(func() {
		runGoCommand = oldRunGoCommand
		runGitCommand = oldRunGitCommand
	})
	runGoCommand = func(root string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if strings.Join(args, " ") == "mod tidy -diff" {
			return []byte("go.mod/go.sum drift"), errors.New("tidy drift")
		}
		return nil, nil
	}
	runGitCommand = func(actualRoot string, args ...string) ([]byte, error) {
		if actualRoot != root {
			t.Fatalf("unexpected root: got %q want %q", actualRoot, root)
		}
		if strings.Join(args, " ") != "ls-files --error-unmatch go.sum" {
			t.Fatalf("unexpected git command: %v", args)
		}
		return nil, nil
	}

	err := run(root)
	if err == nil {
		t.Fatalf("expected go mod tidy -diff drift to fail")
	}
	if !strings.Contains(err.Error(), "go mod tidy -diff") {
		t.Fatalf("expected error to mention go mod tidy -diff, got %v", err)
	}
	wantCalls := []string{"mod verify", "mod tidy -diff"}
	if len(calls) != len(wantCalls) {
		t.Fatalf("unexpected go command calls: got %#v want %#v", calls, wantCalls)
	}
}

func sampleToolchainConfig() toolchainConfig {
	return toolchainConfig{
		GoVersion:    "1.26.1",
		GoModVersion: "1.26.0",
		GoBuildImage: "golang:1.26.1-alpine3.23",
		RuntimeImage: "alpine:3.23",
	}
}

func sampleToolchainFiles(cfg toolchainConfig) map[string]string {
	return map[string]string{
		"go.mod":                                  "module github.com/example/project\n\ngo " + cfg.GoModVersion + "\ntoolchain go" + cfg.GoVersion + "\n",
		"Dockerfile":                              "ARG GO_BUILD_IMAGE=" + cfg.GoBuildImage + "\nARG RUNTIME_IMAGE=" + cfg.RuntimeImage + "\nFROM ${GO_BUILD_IMAGE} AS build\nFROM ${RUNTIME_IMAGE}\n",
		".github/workflows/ci.yml":                "steps:\n  - name: Load toolchain versions\n    run: |\n      while IFS= read -r line; do\n        echo \"$line\" >> \"$GITHUB_ENV\"\n      done < .toolchain.env\n  - uses: actions/setup-go@v5\n    with:\n      go-version: ${{ env.GO_VERSION }}\n",
		".github/workflows/release.yml":           "permissions:\n  contents: write\nsteps:\n  - name: Load toolchain versions\n    run: |\n      while IFS= read -r line; do\n        echo \"$line\" >> \"$GITHUB_ENV\"\n      done < .toolchain.env\n  - uses: actions/setup-go@v5\n    with:\n      go-version: ${{ env.GO_VERSION }}\n  - uses: softprops/action-gh-release@deadbeef\n    with:\n      prerelease: ${{ startsWith(github.ref_name, 'v0.') }}\n      files: |\n        dist/*.sha256\n",
		"README.md":                               "make toolchain-guard\n.toolchain.env\nLocal release packaging helper\nrequires a clean git checkout at the exact tagged release commit\n",
		"docs/operations/TESTING.md":              "make toolchain-guard\n.toolchain.env\n",
		"docs/operations/RELEASE.md":              "make toolchain-guard\n.toolchain.env\nGitHub release workflow publishes the tarball, checksum sidecar, and fsync report as release assets for tagged versions.\n`v0.x` tags are published as prereleases/betas\n`v1.x` tags as normal releases\nrefuses to build from a dirty checkout\ncmd/promptlock-pty-runner\npromptlock-mcp-launch\nLICENSE\nsha256\nGo-native PTY helper\n",
		"docs/standards/ENGINEERING-STANDARDS.md": "make toolchain-guard\n.toolchain.env\n",
		"scripts/run_real_e2e_smoke.sh":           "go build -o \"$PROMPTLOCK_BIN\" ./cmd/promptlock\ngo build -o \"$PTY_RUNNER\" ./cmd/promptlock-pty-runner\n--inputs\npromptlock-pty-runner\n",
		"docs/operations/WRAPPER-EXEC.md":         "--docker-arg as a narrow escape hatch\nhost-alias broker URL\ncontainer could redirect PromptLock transport\n",
		"scripts/release-package.sh":              "require_clean_worktree\nclean git checkout; refusing to build from a dirty tree\ngo run \"github.com/goreleaser/goreleaser/v2@v2.7.0\" build --clean --config .goreleaser.yaml\ncp LICENSE README.md \"$OUT_DIR/\"\npromptlock-mcp-launch-linux-amd64\npromptlock-mcp-launch-darwin-arm64\n",
		".goreleaser.yaml":                        "cmd/promptlock-mcp-launch\npromptlock-mcp-launch-{{ .Os }}-{{ .Arch }}\n-X main.version={{ .Version }}\n",
		"SECURITY.md":                             "latest `main` branch\nOnce prerelease tags exist, fixes are also applied to the latest tagged prerelease.\n",
	}
}

func writeToolchainFixtures(t *testing.T, root string, cfg toolchainConfig) {
	t.Helper()
	env := strings.Join([]string{
		"GO_VERSION=" + cfg.GoVersion,
		"GO_MOD_VERSION=" + cfg.GoModVersion,
		"GO_BUILD_IMAGE=" + cfg.GoBuildImage,
		"RUNTIME_IMAGE=" + cfg.RuntimeImage,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ".toolchain.env"), []byte(env), 0o644); err != nil {
		t.Fatalf("write .toolchain.env: %v", err)
	}
	for path, content := range sampleToolchainFiles(cfg) {
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}
