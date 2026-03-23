package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = orig
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stderr: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return buf.String()
}

func TestFatalSanitizesErrorOutput(t *testing.T) {
	origExit := exitProcess
	exitCode := -1
	exitProcess = func(code int) {
		exitCode = code
	}
	t.Cleanup(func() {
		exitProcess = origExit
	})

	out := captureStderr(t, func() {
		fatal(errors.New("fatal:\x1b[31mboom\x1b[0m"))
	})

	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("expected fatal output to sanitize escape sequences, got %q", out)
	}
	if !strings.Contains(out, `error: fatal:\x1b[31mboom\x1b[0m`) {
		t.Fatalf("expected sanitized fatal output, got %q", out)
	}
}
