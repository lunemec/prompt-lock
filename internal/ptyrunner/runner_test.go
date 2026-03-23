package ptyrunner

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunStagesKeystrokesAndCapturesTranscript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty runner is unix-only")
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "transcript.txt")
	script := `printf 'Actions:\n'; IFS= read -r first; printf 'approved:\n'; IFS= read -r second; printf 'done:%s:%s\n' "$first" "$second"`
	err := Run(Options{
		OutputPath: out,
		Timeout:    5 * time.Second,
		Command:    []string{"sh", "-c", script},
		Stages: []Stage{
			{Triggers: []string{"Actions:"}, Keys: []byte("y\n"), Delay: 10 * time.Millisecond},
			{Triggers: []string{"approved:"}, Keys: []byte("q\n"), Delay: 10 * time.Millisecond},
		},
	})
	if err != nil {
		skipIfPTYUnavailable(t, err)
		t.Fatalf("Run() error = %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	got := string(body)
	for _, want := range []string{"Actions:", "approved:", "done:y:q"} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript %q missing %q", got, want)
		}
	}
}

func TestRunReturnsExitCodeForFailedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty runner is unix-only")
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "transcript.txt")
	err := Run(Options{
		OutputPath: out,
		Timeout:    5 * time.Second,
		Command:    []string{"sh", "-c", "printf 'Actions:\\n'; exit 23"},
		Stages: []Stage{
			{Triggers: []string{"Actions:"}, Keys: []byte("y\n"), Delay: 10 * time.Millisecond},
		},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
	skipIfPTYUnavailable(t, err)
	if exitErr, ok := err.(*ExitError); ok && exitErr.Code != 23 {
		t.Fatalf("exit code = %d, want 23", exitErr.Code)
	}
}

func skipIfPTYUnavailable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if errors.Is(err, os.ErrPermission) || strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "fork/exec") || strings.Contains(msg, "pty") {
		t.Skipf("pty runner unavailable in this environment: %v", err)
	}
}
