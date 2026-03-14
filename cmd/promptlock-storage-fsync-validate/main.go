package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/fsyncreport"
	"github.com/lunemec/promptlock/internal/sopsenv"
)

var loadSOPSEnvFile = sopsenv.LoadFromFile

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("promptlock-storage-fsync-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		reportPath           string
		sopsEnvFile          string
		hmacKeyEnv           string
		hmacKeyIDEnv         string
		hmacKeyringEnv       string
		hmacOverlapWindowEnv string
	)
	fs.StringVar(&reportPath, "file", "", "path to storage fsync report JSON")
	fs.StringVar(&sopsEnvFile, "sops-env-file", "", "optional path to SOPS-encrypted env/json file for key material (falls back to PROMPTLOCK_SOPS_ENV_FILE)")
	fs.StringVar(&hmacKeyEnv, "hmac-key-env", fsyncreport.DefaultHMACKeyEnv, "environment variable name containing the report HMAC key")
	fs.StringVar(&hmacKeyIDEnv, "hmac-key-id-env", fsyncreport.DefaultHMACKeyIDEnv, "environment variable name containing the report HMAC key id")
	fs.StringVar(&hmacKeyringEnv, "hmac-keyring-env", fsyncreport.DefaultHMACKeyringEnv, "environment variable name containing optional verification keyring entries (<key_id>:<env_var>,...)")
	fs.StringVar(&hmacOverlapWindowEnv, "hmac-key-overlap-max-age-env", fsyncreport.DefaultHMACKeyOverlapMaxAgeEnv, "environment variable name containing allowed overlap duration for non-primary verification keys (for example 24h)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reportPath = strings.TrimSpace(reportPath)
	if reportPath == "" {
		fmt.Fprintln(stderr, "Usage: promptlock-storage-fsync-validate --file /path/to/storage-fsync-report.json")
		return 2
	}
	if err := loadSOPSEnvForRun(sopsEnvFile, []string{
		resolveEnvName(hmacKeyEnv, fsyncreport.DefaultHMACKeyEnv),
		resolveEnvName(hmacKeyIDEnv, fsyncreport.DefaultHMACKeyIDEnv),
	}); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	keyring, err := fsyncreport.ResolveVerificationKeyringFromEnv(hmacKeyEnv, hmacKeyIDEnv, hmacKeyringEnv)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	overlapMaxAge, err := resolveOverlapMaxAgeFromEnv(hmacOverlapWindowEnv)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	if err := validateReportFile(reportPath, keyring, overlapMaxAge, time.Now); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "storage fsync report validation passed")
	return 0
}

func resolveEnvName(raw string, fallback string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return v
	}
	return fallback
}

func resolveSOPSEnvFilePath(flagPath string) string {
	if v := strings.TrimSpace(flagPath); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv(sopsenv.DefaultEnvFileEnv))
}

func loadSOPSEnvForRun(flagPath string, requiredKeys []string) error {
	return loadSOPSEnvFile(resolveSOPSEnvFilePath(flagPath), requiredKeys)
}

func resolveOverlapMaxAgeFromEnv(overlapEnv string) (time.Duration, error) {
	envName := strings.TrimSpace(overlapEnv)
	if envName == "" {
		envName = fsyncreport.DefaultHMACKeyOverlapMaxAgeEnv
	}
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return 0, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid overlap duration env %s: %w", envName, err)
	}
	if dur < 0 {
		return 0, fmt.Errorf("invalid overlap duration env %s: must be >= 0", envName)
	}
	return dur, nil
}

func validateReportFile(path string, keyring fsyncreport.VerificationKeyring, overlapMaxAge time.Duration, nowFn func() time.Time) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read report %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var report fsyncreport.Report
	if err := dec.Decode(&report); err != nil {
		return fmt.Errorf("parse report %s: %w", path, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("parse report %s: expected single JSON object", path)
	}
	if err := validateReport(report, keyring, overlapMaxAge, nowFn); err != nil {
		return fmt.Errorf("validate report %s: %w", path, err)
	}
	return nil
}

func validateReport(report fsyncreport.Report, keyring fsyncreport.VerificationKeyring, overlapMaxAge time.Duration, nowFn func() time.Time) error {
	if strings.TrimSpace(report.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if report.SchemaVersion != fsyncreport.SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q", report.SchemaVersion)
	}
	if strings.TrimSpace(report.GeneratedBy) == "" {
		return fmt.Errorf("generated_by is required")
	}
	if report.GeneratedBy != fsyncreport.GeneratedBy {
		return fmt.Errorf("unexpected generated_by %q", report.GeneratedBy)
	}
	if strings.TrimSpace(report.Hostname) == "" {
		return fmt.Errorf("hostname is required")
	}
	if strings.TrimSpace(report.GeneratedAt) == "" {
		return fmt.Errorf("generated_at is required")
	}
	generatedAt, err := time.Parse(time.RFC3339, report.GeneratedAt)
	if err != nil {
		return fmt.Errorf("generated_at must be RFC3339: %w", err)
	}
	if err := fsyncreport.VerifyReportSignatureWithKeyring(report, keyring); err != nil {
		return err
	}
	if err := validateOverlapWindow(report, keyring, generatedAt, overlapMaxAge, nowFn); err != nil {
		return err
	}
	if len(report.Results) == 0 {
		return fmt.Errorf("results must contain at least one mount check")
	}

	seen := make(map[string]struct{}, len(report.Results))
	failedMounts := make([]string, 0)
	allResultsOK := true
	for idx, result := range report.Results {
		dir := strings.TrimSpace(result.Dir)
		if dir == "" {
			return fmt.Errorf("results[%d] missing dir", idx)
		}
		if _, ok := seen[dir]; ok {
			return fmt.Errorf("duplicate dir in results: %s", dir)
		}
		seen[dir] = struct{}{}

		if result.OK {
			continue
		}
		allResultsOK = false
		detail := strings.TrimSpace(result.Error)
		if detail == "" {
			detail = "mount check not OK"
		}
		failedMounts = append(failedMounts, fmt.Sprintf("%s (%s)", dir, detail))
	}

	if len(failedMounts) > 0 {
		return fmt.Errorf("one or more mount checks not OK: %s", strings.Join(failedMounts, "; "))
	}
	if report.OK != allResultsOK {
		return fmt.Errorf("top-level ok=%v does not match result status=%v", report.OK, allResultsOK)
	}
	return nil
}

func validateOverlapWindow(report fsyncreport.Report, keyring fsyncreport.VerificationKeyring, generatedAt time.Time, overlapMaxAge time.Duration, nowFn func() time.Time) error {
	signatureKeyID := strings.TrimSpace(report.Signature.KeyID)
	primaryKeyID := strings.TrimSpace(keyring.PrimaryKeyID)
	if signatureKeyID == primaryKeyID {
		return nil
	}
	if overlapMaxAge <= 0 {
		return fmt.Errorf("signature.key_id %q is not active key %q and overlap window is disabled", signatureKeyID, primaryKeyID)
	}
	currentTime := time.Now
	if nowFn != nil {
		currentTime = nowFn
	}
	now := currentTime().UTC()
	if generatedAt.After(now) {
		return fmt.Errorf("generated_at %q is in the future for rotated key %q", report.GeneratedAt, signatureKeyID)
	}
	age := now.Sub(generatedAt)
	if age > overlapMaxAge {
		return fmt.Errorf("signature.key_id %q exceeds overlap window (%s > %s)", signatureKeyID, age.Round(time.Second), overlapMaxAge)
	}
	return nil
}
