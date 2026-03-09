package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validReportMetadata = `
  "schema_version": "v1",
  "generated_at": "2026-03-09T00:00:00Z",
  "generated_by": "promptlock-storage-fsync-check",
  "hostname": "release-runner"
`

func TestRunUsageErrorWhenFileMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("expected usage text on stderr, got %q", stderr.String())
	}
}

func TestRunFailsWhenReportFileMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing.json")

	code := run([]string{"--file", missing}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "ERROR:") {
		t.Fatalf("expected error output, got %q", stderr.String())
	}
}

func TestRunFailsWhenJSONMalformed(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, "{not-json")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "ERROR:") {
		t.Fatalf("expected parse error output, got %q", stderr.String())
	}
}

func TestRunFailsWhenTopLevelOKFalse(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
`+validReportMetadata+`,
  "ok": false,
  "results": [
    {"dir":"/var/lib/promptlock","ok":false,"error":"fsync directory failed"}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "not OK") {
		t.Fatalf("expected not-ok message, got %q", stderr.String())
	}
}

func TestRunFailsWhenTopLevelStatusMismatchesResults(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
`+validReportMetadata+`,
  "ok": false,
  "results": [
    {"dir":"/var/lib/promptlock","ok":true}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "top-level ok=") {
		t.Fatalf("expected top-level mismatch message, got %q", stderr.String())
	}
}

func TestRunFailsWhenAnyMountNotOK(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
`+validReportMetadata+`,
  "ok": true,
  "results": [
    {"dir":"/var/lib/promptlock","ok":true},
    {"dir":"/var/log/promptlock","ok":false,"error":"fsync directory failed"}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "/var/log/promptlock") {
		t.Fatalf("expected failing mount path in error, got %q", stderr.String())
	}
}

func TestRunFailsWhenDuplicateDirsPresent(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
`+validReportMetadata+`,
  "ok": true,
  "results": [
    {"dir":"/var/lib/promptlock","ok":true},
    {"dir":"/var/lib/promptlock","ok":true}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "duplicate dir") {
		t.Fatalf("expected duplicate dir validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenResultsEmpty(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
`+validReportMetadata+`,
  "ok": true,
  "results": []
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "results") {
		t.Fatalf("expected results validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenResultDirMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
`+validReportMetadata+`,
  "ok": true,
  "results": [
    {"dir":"","ok":true}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "dir") {
		t.Fatalf("expected dir validation error, got %q", stderr.String())
	}
}

func TestRunSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
`+validReportMetadata+`,
  "ok": true,
  "results": [
    {"dir":"/var/lib/promptlock","ok":true},
    {"dir":"/var/log/promptlock","ok":true}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "validation passed") {
		t.Fatalf("expected success output, got %q", stdout.String())
	}
}

func TestRunFailsWhenSchemaVersionMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
  "generated_at": "2026-03-09T00:00:00Z",
  "generated_by": "promptlock-storage-fsync-check",
  "hostname": "release-runner",
  "ok": true,
  "results": [
    {"dir":"/var/lib/promptlock","ok":true}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "schema_version") {
		t.Fatalf("expected schema validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenGeneratedByUnexpected(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
  "schema_version": "v1",
  "generated_at": "2026-03-09T00:00:00Z",
  "generated_by": "unknown-generator",
  "hostname": "release-runner",
  "ok": true,
  "results": [
    {"dir":"/var/lib/promptlock","ok":true}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "generated_by") {
		t.Fatalf("expected generated_by validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenGeneratedAtInvalid(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeReportFile(t, `{
  "schema_version": "v1",
  "generated_at": "invalid",
  "generated_by": "promptlock-storage-fsync-check",
  "hostname": "release-runner",
  "ok": true,
  "results": [
    {"dir":"/var/lib/promptlock","ok":true}
  ]
}`)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "generated_at") {
		t.Fatalf("expected generated_at validation error, got %q", stderr.String())
	}
}

func writeReportFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write report fixture: %v", err)
	}
	return path
}
