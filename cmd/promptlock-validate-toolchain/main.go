package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type toolchainConfig struct {
	GoVersion    string
	GoModVersion string
	GoBuildImage string
	RuntimeImage string
}

var runGoCommand = func(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		return b, fmt.Errorf("%s: %w: %s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(string(b)))
	}
	return b, nil
}

var runGitCommand = func(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		return b, fmt.Errorf("%s: %w: %s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(string(b)))
	}
	return b, nil
}

func main() {
	root := "."
	if len(os.Args) > 2 {
		fmt.Println("Usage: go run ./cmd/promptlock-validate-toolchain [repo-root]")
		os.Exit(1)
	}
	if len(os.Args) == 2 {
		root = os.Args[1]
	}

	if err := run(root); err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Toolchain version validation passed")
}

func run(root string) error {
	cfg, err := loadToolchainConfig(filepath.Join(root, ".toolchain.env"))
	if err != nil {
		return err
	}

	files := map[string]string{}
	for _, rel := range []string{
		"go.mod",
		"Dockerfile",
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
		"README.md",
		"docs/operations/TESTING.md",
		"docs/operations/RELEASE.md",
		"docs/operations/WRAPPER-EXEC.md",
		"docs/standards/ENGINEERING-STANDARDS.md",
		"scripts/run_real_e2e_smoke.sh",
		"scripts/release-package.sh",
		".goreleaser.yaml",
		"SECURITY.md",
	} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		files[rel] = string(b)
	}

	if err := validateToolchainFiles(cfg, files); err != nil {
		return err
	}
	if err := validateGoSumTracked(root); err != nil {
		return err
	}
	if err := validateModuleReproducibility(root); err != nil {
		return err
	}
	return nil
}

func loadToolchainConfig(path string) (toolchainConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return toolchainConfig{}, fmt.Errorf(".toolchain.env: %w", err)
	}

	values := map[string]string{}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return toolchainConfig{}, fmt.Errorf(".toolchain.env: invalid line %q", raw)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	cfg := toolchainConfig{
		GoVersion:    values["GO_VERSION"],
		GoModVersion: values["GO_MOD_VERSION"],
		GoBuildImage: values["GO_BUILD_IMAGE"],
		RuntimeImage: values["RUNTIME_IMAGE"],
	}
	if cfg.GoVersion == "" || cfg.GoModVersion == "" || cfg.GoBuildImage == "" || cfg.RuntimeImage == "" {
		return toolchainConfig{}, fmt.Errorf(".toolchain.env: GO_VERSION, GO_MOD_VERSION, GO_BUILD_IMAGE, and RUNTIME_IMAGE are required")
	}
	return cfg, nil
}

func validateToolchainFiles(cfg toolchainConfig, files map[string]string) error {
	if err := validateGoMod(files["go.mod"], cfg); err != nil {
		return fmt.Errorf("go.mod: %w", err)
	}
	if err := validateDockerfile(files["Dockerfile"], cfg); err != nil {
		return fmt.Errorf("Dockerfile: %w", err)
	}
	if err := validateWorkflow(files[".github/workflows/ci.yml"]); err != nil {
		return fmt.Errorf(".github/workflows/ci.yml: %w", err)
	}
	if err := validateWorkflow(files[".github/workflows/release.yml"]); err != nil {
		return fmt.Errorf(".github/workflows/release.yml: %w", err)
	}
	if err := validateDocs(files); err != nil {
		return err
	}
	if err := validateReleasePackaging(files); err != nil {
		return err
	}
	return nil
}

func validateGoMod(text string, cfg toolchainConfig) error {
	goVersion, err := captureSingle(text, `(?m)^go\s+([^\s]+)\s*$`)
	if err != nil {
		return fmt.Errorf("missing go directive")
	}
	if goVersion != cfg.GoModVersion {
		return fmt.Errorf("go directive = %q, want %q", goVersion, cfg.GoModVersion)
	}

	toolchainVersion, err := captureSingle(text, `(?m)^toolchain\s+([^\s]+)\s*$`)
	if err != nil {
		return fmt.Errorf("missing toolchain directive")
	}
	wantToolchain := "go" + cfg.GoVersion
	if toolchainVersion != wantToolchain {
		return fmt.Errorf("toolchain directive = %q, want %q", toolchainVersion, wantToolchain)
	}
	return nil
}

func validateDockerfile(text string, cfg toolchainConfig) error {
	required := []string{
		"ARG GO_BUILD_IMAGE=" + cfg.GoBuildImage,
		"ARG RUNTIME_IMAGE=" + cfg.RuntimeImage,
		"FROM ${GO_BUILD_IMAGE} AS build",
		"FROM ${RUNTIME_IMAGE}",
	}
	for _, needle := range required {
		if !strings.Contains(text, needle) {
			return fmt.Errorf("missing %q", needle)
		}
	}
	return nil
}

func validateWorkflow(text string) error {
	if !strings.Contains(text, ".toolchain.env") {
		return fmt.Errorf("must load .toolchain.env into GITHUB_ENV")
	}
	if strings.Contains(text, "go-version-file:") {
		return fmt.Errorf("must not use go-version-file when toolchain pins are centralized")
	}
	if !strings.Contains(text, "go-version: ${{ env.GO_VERSION }}") {
		return fmt.Errorf("must use go-version from env.GO_VERSION")
	}
	return nil
}

func validateDocs(files map[string]string) error {
	docRequirements := map[string][]string{
		"README.md":                               {"make toolchain-guard", ".toolchain.env"},
		"docs/operations/TESTING.md":              {"make toolchain-guard", ".toolchain.env"},
		"docs/operations/RELEASE.md":              {"make toolchain-guard", ".toolchain.env"},
		"docs/standards/ENGINEERING-STANDARDS.md": {".toolchain.env", "make toolchain-guard"},
	}
	for path, needles := range docRequirements {
		for _, needle := range needles {
			if !strings.Contains(files[path], needle) {
				return fmt.Errorf("%s: missing %q", path, needle)
			}
		}
	}
	return nil
}

func validateReleasePackaging(files map[string]string) error {
	requirements := map[string][]string{
		"scripts/release-package.sh": {"require_clean_worktree", "clean git checkout; refusing to build from a dirty tree", "LICENSE", "promptlock-mcp-launch"},
		".goreleaser.yaml":           {"cmd/promptlock-mcp-launch", "promptlock-mcp-launch-{{ .Os }}-{{ .Arch }}"},
		".github/workflows/release.yml": {
			"softprops/action-gh-release@",
			"permissions:",
			"contents: write",
			"prerelease: ${{ startsWith(github.ref_name, 'v0.') }}",
			"dist/*.sha256",
		},
		"docs/operations/RELEASE.md": {
			"GitHub release workflow publishes the tarball, checksum sidecar, and fsync report as release assets for tagged versions.",
			"`v0.x` tags are published as prereleases/betas",
			"`v1.x` tags as normal releases",
			"refuses to build from a dirty checkout",
			"cmd/promptlock-pty-runner",
			"promptlock-mcp-launch",
			"LICENSE",
			"sha256",
			"Go-native PTY helper",
		},
		"scripts/run_real_e2e_smoke.sh": {
			"go build -o \"$PTY_RUNNER\" ./cmd/promptlock-pty-runner",
			"--inputs",
			"promptlock-pty-runner",
		},
		"docs/operations/WRAPPER-EXEC.md": {
			"host-alias broker URL",
			"container could redirect PromptLock transport",
		},
		"README.md": {
			"Local release packaging helper",
			"requires a clean git checkout at the exact tagged release commit",
		},
		"SECURITY.md": {
			"latest `main` branch",
			"Once prerelease tags exist, fixes are also applied to the latest tagged prerelease.",
		},
	}
	for path, needles := range requirements {
		for _, needle := range needles {
			if !strings.Contains(files[path], needle) {
				return fmt.Errorf("%s: missing %q", path, needle)
			}
		}
	}
	if strings.Contains(files["scripts/run_real_e2e_smoke.sh"], "python") {
		return fmt.Errorf("scripts/run_real_e2e_smoke.sh: must not reference python")
	}
	if strings.Contains(files["docs/operations/RELEASE.md"], "python") {
		return fmt.Errorf("docs/operations/RELEASE.md: must not reference python in the supported smoke path")
	}
	return nil
}

func validateModuleReproducibility(root string) error {
	if _, err := runGoCommand(root, "mod", "verify"); err != nil {
		return fmt.Errorf("go mod verify: %w", err)
	}
	if _, err := runGoCommand(root, "mod", "tidy", "-diff"); err != nil {
		return fmt.Errorf("go mod tidy -diff: %w", err)
	}
	return nil
}

func validateGoSumTracked(root string) error {
	sumPath := filepath.Join(root, "go.sum")
	if _, err := os.Stat(sumPath); err != nil {
		return fmt.Errorf("go.sum: %w", err)
	}
	if _, err := runGitCommand(root, "ls-files", "--error-unmatch", "go.sum"); err != nil {
		return fmt.Errorf("go.sum must be tracked in git: %w", err)
	}
	return nil
}

func captureSingle(text, pattern string) (string, error) {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 2 {
		return "", fmt.Errorf("no match")
	}
	return matches[1], nil
}
