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
	testValidateHMACKeyEnv   = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY"
	testValidateHMACKeyIDEnv = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID"
	testValidateHMACKeyring  = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING"
	testValidateOverlapEnv   = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE"
)

type reportFixture struct {
	SchemaVersion string          `json:"schema_version"`
	GeneratedAt   string          `json:"generated_at"`
	GeneratedBy   string          `json:"generated_by"`
	Hostname      string          `json:"hostname"`
	OK            bool            `json:"ok"`
	Results       []reportResult  `json:"results"`
	Signature     reportSignature `json:"signature"`
}

type reportResult struct {
	Dir   string `json:"dir"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type reportSignature struct {
	Alg   string `json:"alg"`
	KeyID string `json:"key_id"`
	Value string `json:"value"`
}

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
	setValidateKeyEnv(t)

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
	setValidateKeyEnv(t)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "ERROR:") {
		t.Fatalf("expected parse error output, got %q", stderr.String())
	}
}

func TestRunFailsWhenSignatureMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	report := writeUnsignedReportFile(t, defaultValidReport())
	setValidateKeyEnv(t)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "signature") {
		t.Fatalf("expected signature validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenSignatureAlgUnexpected(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	reportData.Signature.Alg = "sha256"
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "signature.alg") {
		t.Fatalf("expected signature algorithm validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenSignatureKeyIDMismatch(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, "old-key")
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "key_id") {
		t.Fatalf("expected signature key_id validation error, got %q", stderr.String())
	}
}

func TestRunSuccessWhenRotatedKeyWithinOverlapWindow(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.GeneratedAt = time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)
	reportData.Signature = signFixtureReport(t, reportData, validatePreviousKeyValue, validatePreviousKeyID)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)
	t.Setenv(testValidateHMACKeyring, validatePreviousKeyID+":"+testValidatePreviousHMACKeyEnv)
	t.Setenv(testValidatePreviousHMACKeyEnv, validatePreviousKeyValue)
	t.Setenv(testValidateOverlapEnv, "1h")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success exit code 0 with rotated key overlap, got %d, stderr=%q", code, stderr.String())
	}
}

func TestRunFailsWhenRotatedKeyOutsideOverlapWindow(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.GeneratedAt = time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)
	reportData.Signature = signFixtureReport(t, reportData, validatePreviousKeyValue, validatePreviousKeyID)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)
	t.Setenv(testValidateHMACKeyring, validatePreviousKeyID+":"+testValidatePreviousHMACKeyEnv)
	t.Setenv(testValidatePreviousHMACKeyEnv, validatePreviousKeyValue)
	t.Setenv(testValidateOverlapEnv, "30m")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1 with expired overlap window, got %d", code)
	}
	if !strings.Contains(stderr.String(), "overlap window") {
		t.Fatalf("expected overlap-window validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenRotatedKeyGeneratedAtFuture(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.GeneratedAt = time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	reportData.Signature = signFixtureReport(t, reportData, validatePreviousKeyValue, validatePreviousKeyID)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)
	t.Setenv(testValidateHMACKeyring, validatePreviousKeyID+":"+testValidatePreviousHMACKeyEnv)
	t.Setenv(testValidatePreviousHMACKeyEnv, validatePreviousKeyValue)
	t.Setenv(testValidateOverlapEnv, "1h")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1 for rotated key with future generated_at, got %d", code)
	}
	if !strings.Contains(stderr.String(), "in the future") {
		t.Fatalf("expected future generated_at validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenRotatedKeyAndOverlapWindowDisabled(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.GeneratedAt = time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	reportData.Signature = signFixtureReport(t, reportData, validatePreviousKeyValue, validatePreviousKeyID)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)
	t.Setenv(testValidateHMACKeyring, validatePreviousKeyID+":"+testValidatePreviousHMACKeyEnv)
	t.Setenv(testValidatePreviousHMACKeyEnv, validatePreviousKeyValue)
	t.Setenv(testValidateOverlapEnv, "")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1 when overlap window disabled, got %d", code)
	}
	if !strings.Contains(stderr.String(), "overlap window is disabled") {
		t.Fatalf("expected disabled overlap-window validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenSignatureKeyIDNotInKeyring(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validatePreviousKeyValue, "unknown-key")
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)
	t.Setenv(testValidateHMACKeyring, validatePreviousKeyID+":"+testValidatePreviousHMACKeyEnv)
	t.Setenv(testValidatePreviousHMACKeyEnv, validatePreviousKeyValue)
	t.Setenv(testValidateOverlapEnv, "1h")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1 for unknown signature key id, got %d", code)
	}
	if !strings.Contains(stderr.String(), "verification keyring") {
		t.Fatalf("expected keyring validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenSignatureValueInvalid(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	reportData.Hostname = "tampered-host"
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "signature verification failed") {
		t.Fatalf("expected signature verification failure, got %q", stderr.String())
	}
}

func TestRunFailsWhenTopLevelOKFalse(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.OK = false
	reportData.Results = []reportResult{{Dir: "/var/lib/promptlock", OK: false, Error: "fsync directory failed"}}
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.OK = false
	reportData.Results = []reportResult{{Dir: "/var/lib/promptlock", OK: true}}
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.OK = true
	reportData.Results = []reportResult{
		{Dir: "/var/lib/promptlock", OK: true},
		{Dir: "/var/log/promptlock", OK: false, Error: "fsync directory failed"},
	}
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.Results = []reportResult{
		{Dir: "/var/lib/promptlock", OK: true},
		{Dir: "/var/lib/promptlock", OK: true},
	}
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.Results = []reportResult{}
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.Results = []reportResult{{Dir: "", OK: true}}
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.SchemaVersion = ""
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.GeneratedBy = "unknown-generator"
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

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
	reportData := defaultValidReport()
	reportData.GeneratedAt = "invalid"
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "generated_at") {
		t.Fatalf("expected generated_at validation error, got %q", stderr.String())
	}
}

func TestRunFailsWhenSigningKeyMissingInEnv(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	t.Setenv(testValidateHMACKeyEnv, "")
	t.Setenv(testValidateHMACKeyIDEnv, validateKeyIDValue)

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage/config exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), testValidateHMACKeyEnv) {
		t.Fatalf("expected missing key env error, got %q", stderr.String())
	}
}

func TestRunFailsWhenSigningKeyIDMissingInEnv(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	t.Setenv(testValidateHMACKeyEnv, validateKeyValue)
	t.Setenv(testValidateHMACKeyIDEnv, "")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage/config exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), testValidateHMACKeyIDEnv) {
		t.Fatalf("expected missing key id env error, got %q", stderr.String())
	}
}

func TestRunLoadsSigningKeyFromSOPSEnvFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	t.Setenv(testValidateHMACKeyEnv, "")
	t.Setenv(testValidateHMACKeyIDEnv, "")

	orig := loadSOPSEnvFile
	t.Cleanup(func() { loadSOPSEnvFile = orig })
	loadSOPSEnvFile = func(path string, requiredKeys []string) error {
		if path != "/tmp/promptlock-fsync.sops.env" {
			t.Fatalf("unexpected sops env path: %q", path)
		}
		if len(requiredKeys) != 2 || requiredKeys[0] != testValidateHMACKeyEnv || requiredKeys[1] != testValidateHMACKeyIDEnv {
			t.Fatalf("unexpected required keys: %#v", requiredKeys)
		}
		t.Setenv(testValidateHMACKeyEnv, validateKeyValue)
		t.Setenv(testValidateHMACKeyIDEnv, validateKeyIDValue)
		return nil
	}

	code := run([]string{"--file", report, "--sops-env-file", "/tmp/promptlock-fsync.sops.env"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "validation passed") {
		t.Fatalf("expected success output, got %q", stdout.String())
	}
}

func TestRunFailsWhenOverlapDurationEnvInvalid(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)
	t.Setenv(testValidateOverlapEnv, "not-a-duration")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage/config exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), testValidateOverlapEnv) {
		t.Fatalf("expected overlap duration env error, got %q", stderr.String())
	}
}

func TestRunFailsWhenOverlapDurationEnvNegative(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reportData := defaultValidReport()
	reportData.Signature = signFixtureReport(t, reportData, validateKeyValue, validateKeyIDValue)
	report := writeSignedReportFile(t, reportData)
	setValidateKeyEnv(t)
	t.Setenv(testValidateOverlapEnv, "-1h")

	code := run([]string{"--file", report}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage/config exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "must be >=") {
		t.Fatalf("expected non-negative overlap duration env error, got %q", stderr.String())
	}
}

const (
	validateKeyValue               = "0123456789abcdef0123456789abcdef"
	validateKeyIDValue             = "release-hmac-key-1"
	validatePreviousKeyValue       = "fedcba9876543210fedcba9876543210"
	validatePreviousKeyID          = "release-hmac-key-0"
	testValidatePreviousHMACKeyEnv = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV"
)

func setValidateKeyEnv(t *testing.T) {
	t.Helper()
	t.Setenv(testValidateHMACKeyEnv, validateKeyValue)
	t.Setenv(testValidateHMACKeyIDEnv, validateKeyIDValue)
}

func defaultValidReport() reportFixture {
	return reportFixture{
		SchemaVersion: "v1",
		GeneratedAt:   "2026-03-09T00:00:00Z",
		GeneratedBy:   "promptlock-storage-fsync-check",
		Hostname:      "release-runner",
		OK:            true,
		Results: []reportResult{
			{Dir: "/var/lib/promptlock", OK: true},
			{Dir: "/var/log/promptlock", OK: true},
		},
	}
}

func signFixtureReport(t *testing.T, report reportFixture, key string, keyID string) reportSignature {
	t.Helper()
	payload, err := json.Marshal(struct {
		SchemaVersion string         `json:"schema_version"`
		GeneratedAt   string         `json:"generated_at"`
		GeneratedBy   string         `json:"generated_by"`
		Hostname      string         `json:"hostname"`
		OK            bool           `json:"ok"`
		Results       []reportResult `json:"results"`
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
		t.Fatalf("sign payload: %v", err)
	}
	return reportSignature{
		Alg:   "hmac-sha256",
		KeyID: keyID,
		Value: base64.StdEncoding.EncodeToString(mac.Sum(nil)),
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

func writeSignedReportFile(t *testing.T, report reportFixture) string {
	t.Helper()
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal signed report fixture: %v", err)
	}
	return writeReportFile(t, string(data))
}

func writeUnsignedReportFile(t *testing.T, report reportFixture) string {
	t.Helper()
	data, err := json.MarshalIndent(struct {
		SchemaVersion string         `json:"schema_version"`
		GeneratedAt   string         `json:"generated_at"`
		GeneratedBy   string         `json:"generated_by"`
		Hostname      string         `json:"hostname"`
		OK            bool           `json:"ok"`
		Results       []reportResult `json:"results"`
	}{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   report.GeneratedAt,
		GeneratedBy:   report.GeneratedBy,
		Hostname:      report.Hostname,
		OK:            report.OK,
		Results:       report.Results,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal unsigned report fixture: %v", err)
	}
	return writeReportFile(t, string(data))
}
