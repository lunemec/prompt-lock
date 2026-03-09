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
)

const (
	expectedReportSchemaVersion = "v1"
	expectedReportGeneratedBy   = "promptlock-storage-fsync-check"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("promptlock-storage-fsync-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var reportPath string
	fs.StringVar(&reportPath, "file", "", "path to storage fsync report JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reportPath = strings.TrimSpace(reportPath)
	if reportPath == "" {
		fmt.Fprintln(stderr, "Usage: promptlock-storage-fsync-validate --file /path/to/storage-fsync-report.json")
		return 2
	}
	if err := validateReportFile(reportPath); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "storage fsync report validation passed")
	return 0
}

type fsyncReport struct {
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	GeneratedBy   string             `json:"generated_by"`
	Hostname      string             `json:"hostname"`
	OK            bool               `json:"ok"`
	Results       []fsyncMountResult `json:"results"`
}

type fsyncMountResult struct {
	Dir   string `json:"dir"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func validateReportFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read report %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var report fsyncReport
	if err := dec.Decode(&report); err != nil {
		return fmt.Errorf("parse report %s: %w", path, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("parse report %s: expected single JSON object", path)
	}
	if err := validateReport(report); err != nil {
		return fmt.Errorf("validate report %s: %w", path, err)
	}
	return nil
}

func validateReport(report fsyncReport) error {
	if strings.TrimSpace(report.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if report.SchemaVersion != expectedReportSchemaVersion {
		return fmt.Errorf("unsupported schema_version %q", report.SchemaVersion)
	}
	if strings.TrimSpace(report.GeneratedBy) == "" {
		return fmt.Errorf("generated_by is required")
	}
	if report.GeneratedBy != expectedReportGeneratedBy {
		return fmt.Errorf("unexpected generated_by %q", report.GeneratedBy)
	}
	if strings.TrimSpace(report.Hostname) == "" {
		return fmt.Errorf("hostname is required")
	}
	if strings.TrimSpace(report.GeneratedAt) == "" {
		return fmt.Errorf("generated_at is required")
	}
	if _, err := time.Parse(time.RFC3339, report.GeneratedAt); err != nil {
		return fmt.Errorf("generated_at must be RFC3339: %w", err)
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
