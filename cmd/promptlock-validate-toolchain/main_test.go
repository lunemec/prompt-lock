package main

import "testing"

func TestValidateToolchainFilesPasses(t *testing.T) {
	cfg := sampleToolchainConfig()
	if err := validateToolchainFiles(cfg, sampleToolchainFiles(cfg)); err != nil {
		t.Fatalf("expected validation to pass, got %v", err)
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
		".github/workflows/release.yml":           "steps:\n  - name: Load toolchain versions\n    run: |\n      while IFS= read -r line; do\n        echo \"$line\" >> \"$GITHUB_ENV\"\n      done < .toolchain.env\n  - uses: actions/setup-go@v5\n    with:\n      go-version: ${{ env.GO_VERSION }}\n",
		"README.md":                               "make toolchain-guard\n.toolchain.env\n",
		"docs/operations/TESTING.md":              "make toolchain-guard\n.toolchain.env\n",
		"docs/operations/RELEASE.md":              "make toolchain-guard\n.toolchain.env\n",
		"docs/standards/ENGINEERING-STANDARDS.md": "make toolchain-guard\n.toolchain.env\n",
	}
}
