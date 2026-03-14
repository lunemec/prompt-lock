package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testReportHMACKeyEnv   = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY"
	testReportHMACKeyIDEnv = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID"
)

func TestRunUsageErrorWhenDirMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2 for usage error, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("expected usage text on stderr, got %q", stderr.String())
	}
}

func TestRunSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := t.TempDir()

	code := run([]string{"--dir", dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "OK: file+directory fsync succeeded") {
		t.Fatalf("expected success output, got %q", stdout.String())
	}
	finalPath := filepath.Join(dir, ".promptlock-fsync-check")
	if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
		t.Fatalf("expected probe file cleanup, stat err=%v", err)
	}
}

func TestRunFailsWhenDirPathIsAFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	baseDir := t.TempDir()
	filePath := filepath.Join(baseDir, "not-a-directory")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	code := run([]string{"--dir", filePath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "ERROR:") {
		t.Fatalf("expected error output, got %q", stderr.String())
	}
}

type jsonReportResult struct {
	Dir   string `json:"dir"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type jsonReportSignature struct {
	Alg   string `json:"alg"`
	KeyID string `json:"key_id"`
	Value string `json:"value"`
}

type jsonReport struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	GeneratedBy   string              `json:"generated_by"`
	Hostname      string              `json:"hostname"`
	OK            bool                `json:"ok"`
	Results       []jsonReportResult  `json:"results"`
	Signature     jsonReportSignature `json:"signature"`
}

func TestRunJSONReportAllSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dirA := t.TempDir()
	dirB := t.TempDir()
	key := "0123456789abcdef0123456789abcdef"
	keyID := "release-hmac-key-1"
	t.Setenv(testReportHMACKeyEnv, key)
	t.Setenv(testReportHMACKeyIDEnv, keyID)

	code := run([]string{"--dir", dirA, "--dir", dirB, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	var out jsonReport
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("parse json report: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok=true report, got false")
	}
	if out.SchemaVersion != "v1" {
		t.Fatalf("expected schema_version=v1, got %q", out.SchemaVersion)
	}
	if out.GeneratedBy != "promptlock-storage-fsync-check" {
		t.Fatalf("expected generated_by promptlock-storage-fsync-check, got %q", out.GeneratedBy)
	}
	if out.Hostname == "" {
		t.Fatalf("expected hostname to be present")
	}
	if _, err := time.Parse(time.RFC3339, out.GeneratedAt); err != nil {
		t.Fatalf("expected generated_at in RFC3339 format, got %q (%v)", out.GeneratedAt, err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out.Results))
	}
	if out.Signature.Alg != "hmac-sha256" {
		t.Fatalf("expected signature.alg hmac-sha256, got %q", out.Signature.Alg)
	}
	if out.Signature.KeyID != keyID {
		t.Fatalf("expected signature.key_id %q, got %q", keyID, out.Signature.KeyID)
	}
	if out.Signature.Value == "" {
		t.Fatalf("expected signature.value")
	}
	if _, err := base64.StdEncoding.DecodeString(out.Signature.Value); err != nil {
		t.Fatalf("expected base64 signature value, got %q (%v)", out.Signature.Value, err)
	}
	expectedSig := computeReportHMACValue(t, key, out)
	if out.Signature.Value != expectedSig {
		t.Fatalf("expected deterministic signature value %q, got %q", expectedSig, out.Signature.Value)
	}
}

func TestRunJSONReportPartialFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	okDir := t.TempDir()
	baseDir := t.TempDir()
	badPath := filepath.Join(baseDir, "not-a-directory")
	if err := os.WriteFile(badPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	t.Setenv(testReportHMACKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(testReportHMACKeyIDEnv, "release-hmac-key-1")

	code := run([]string{"--dir", okDir, "--dir", badPath, "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1 for partial failure, got %d", code)
	}
	var out jsonReport
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("parse json report: %v", err)
	}
	if out.OK {
		t.Fatalf("expected ok=false report when one mount fails")
	}
	if len(out.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out.Results))
	}
	failed := false
	for _, r := range out.Results {
		if r.Dir == badPath {
			failed = true
			if r.OK {
				t.Fatalf("expected failing mount to report ok=false")
			}
			if r.Error == "" {
				t.Fatalf("expected failing mount to include error detail")
			}
		}
	}
	if !failed {
		t.Fatalf("expected report to include failing mount %q", badPath)
	}
}

func TestRunJSONReportWithDirListFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dirA := t.TempDir()
	dirB := t.TempDir()
	dirList := dirA + "," + dirB
	t.Setenv(testReportHMACKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(testReportHMACKeyIDEnv, "release-hmac-key-1")

	code := run([]string{"--dir-list", dirList, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	var out jsonReport
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("parse json report: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok=true report for dir-list")
	}
	if len(out.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out.Results))
	}
}

func TestRunJSONReportFailsWhenSigningKeyMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := t.TempDir()
	t.Setenv(testReportHMACKeyEnv, "")
	t.Setenv(testReportHMACKeyIDEnv, "release-hmac-key-1")

	code := run([]string{"--dir", dir, "--json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage/config exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), testReportHMACKeyEnv) {
		t.Fatalf("expected missing key env error, got %q", stderr.String())
	}
}

func TestRunJSONReportFailsWhenSigningKeyIDMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := t.TempDir()
	t.Setenv(testReportHMACKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(testReportHMACKeyIDEnv, "")

	code := run([]string{"--dir", dir, "--json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage/config exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), testReportHMACKeyIDEnv) {
		t.Fatalf("expected missing key id env error, got %q", stderr.String())
	}
}

func TestRunJSONReportLoadsKeysFromSOPSEnvFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := t.TempDir()
	t.Setenv(testReportHMACKeyEnv, "")
	t.Setenv(testReportHMACKeyIDEnv, "")

	orig := loadSOPSEnvFile
	t.Cleanup(func() { loadSOPSEnvFile = orig })
	loadSOPSEnvFile = func(path string, requiredKeys []string) error {
		if path != "/tmp/promptlock-fsync.sops.env" {
			t.Fatalf("unexpected sops env path: %q", path)
		}
		if len(requiredKeys) != 2 || requiredKeys[0] != testReportHMACKeyEnv || requiredKeys[1] != testReportHMACKeyIDEnv {
			t.Fatalf("unexpected required keys: %#v", requiredKeys)
		}
		t.Setenv(testReportHMACKeyEnv, "0123456789abcdef0123456789abcdef")
		t.Setenv(testReportHMACKeyIDEnv, "release-hmac-key-1")
		return nil
	}

	code := run([]string{"--dir", dir, "--json", "--sops-env-file", "/tmp/promptlock-fsync.sops.env"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatalf("expected json output")
	}
}

func computeReportHMACValue(t *testing.T, key string, report jsonReport) string {
	t.Helper()
	payload, err := json.Marshal(struct {
		SchemaVersion string             `json:"schema_version"`
		GeneratedAt   string             `json:"generated_at"`
		GeneratedBy   string             `json:"generated_by"`
		Hostname      string             `json:"hostname"`
		OK            bool               `json:"ok"`
		Results       []jsonReportResult `json:"results"`
	}{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   report.GeneratedAt,
		GeneratedBy:   report.GeneratedBy,
		Hostname:      report.Hostname,
		OK:            report.OK,
		Results:       report.Results,
	})
	if err != nil {
		t.Fatalf("marshal signing payload: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(key))
	if _, err := mac.Write(payload); err != nil {
		t.Fatalf("write signing payload: %v", err)
	}
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
