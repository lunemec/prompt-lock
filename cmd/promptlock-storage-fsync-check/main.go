package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	reportSchemaVersion = "v1"
	reportGeneratedBy   = "promptlock-storage-fsync-check"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("promptlock-storage-fsync-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		dirs    stringListFlag
		dirList string
		jsonOut bool
	)
	fs.Var(&dirs, "dir", "target mount directory to verify (repeatable)")
	fs.StringVar(&dirList, "dir-list", "", "comma-separated mount directories to verify")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON report for one or more directories")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	targetDirs := collectDirs([]string(dirs), dirList)
	if len(targetDirs) == 0 {
		fmt.Fprintln(stderr, "Usage: promptlock-storage-fsync-check --dir /path/to/mount")
		return 2
	}
	results := make([]checkResult, 0, len(targetDirs))
	allOK := true
	for _, dir := range targetDirs {
		if err := checkMountFSync(dir); err != nil {
			allOK = false
			results = append(results, checkResult{Dir: dir, OK: false, Error: err.Error()})
			continue
		}
		results = append(results, checkResult{Dir: dir, OK: true})
	}
	if jsonOut {
		hostname, err := os.Hostname()
		if err != nil || strings.TrimSpace(hostname) == "" {
			hostname = "unknown-host"
		}
		report := checkReport{
			SchemaVersion: reportSchemaVersion,
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			GeneratedBy:   reportGeneratedBy,
			Hostname:      hostname,
			OK:            allOK,
			Results:       results,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "ERROR: encode json report: %v\n", err)
			return 1
		}
		if allOK {
			return 0
		}
		return 1
	}
	for _, r := range results {
		if r.OK {
			fmt.Fprintf(stdout, "OK: file+directory fsync succeeded for %s\n", r.Dir)
			continue
		}
		fmt.Fprintf(stderr, "ERROR: %s\n", r.Error)
	}
	if allOK {
		return 0
	}
	return 1
}

func checkMountFSync(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prepare mount dir %s: %w", dir, err)
	}
	tmpFile, err := os.CreateTemp(dir, ".promptlock-fsync-check-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmpFile.Write([]byte("promptlock-fsync-check\n")); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file %s: %w", tmpName, err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("fsync temp file %s: %w", tmpName, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmpName, err)
	}

	finalPath := filepath.Join(dir, ".promptlock-fsync-check")
	if err := os.Rename(tmpName, finalPath); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpName, finalPath, err)
	}
	cleanup = false

	dirHandle, err := os.Open(dir)
	if err != nil {
		_ = os.Remove(finalPath)
		return fmt.Errorf("open directory %s for fsync: %w", dir, err)
	}
	if err := dirHandle.Sync(); err != nil {
		_ = dirHandle.Close()
		_ = os.Remove(finalPath)
		return fmt.Errorf("fsync directory %s: %w", dir, err)
	}
	if err := dirHandle.Close(); err != nil {
		_ = os.Remove(finalPath)
		return fmt.Errorf("close directory handle %s: %w", dir, err)
	}
	if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleanup probe file %s: %w", finalPath, err)
	}
	return nil
}

type stringListFlag []string

func (s *stringListFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringListFlag) Set(value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("dir cannot be empty")
	}
	*s = append(*s, v)
	return nil
}

func collectDirs(flagDirs []string, dirList string) []string {
	out := make([]string, 0, len(flagDirs)+1)
	for _, d := range flagDirs {
		if v := strings.TrimSpace(d); v != "" {
			out = append(out, v)
		}
	}
	for _, d := range strings.Split(dirList, ",") {
		if v := strings.TrimSpace(d); v != "" {
			out = append(out, v)
		}
	}
	return out
}

type checkResult struct {
	Dir   string `json:"dir"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type checkReport struct {
	SchemaVersion string        `json:"schema_version"`
	GeneratedAt   string        `json:"generated_at"`
	GeneratedBy   string        `json:"generated_by"`
	Hostname      string        `json:"hostname"`
	OK            bool          `json:"ok"`
	Results       []checkResult `json:"results"`
}
