package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

type jsonReport struct {
	SchemaVersion string `json:"schema_version"`
	GeneratedAt   string `json:"generated_at"`
	GeneratedBy   string `json:"generated_by"`
	Hostname      string `json:"hostname"`
	OK            bool   `json:"ok"`
	Results       []struct {
		Dir   string `json:"dir"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	} `json:"results"`
}

func TestRunJSONReportAllSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dirA := t.TempDir()
	dirB := t.TempDir()

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
