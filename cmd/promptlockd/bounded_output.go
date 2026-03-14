package main

import (
	"bytes"
	"io"
	"os/exec"
	"sync"
)

const defaultOutputCaptureBytes = 64 * 1024

type boundedCaptureWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func newBoundedCaptureWriter(limit int) *boundedCaptureWriter {
	if limit < 0 {
		limit = 0
	}
	return &boundedCaptureWriter{limit: limit}
}

func (w *boundedCaptureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.limit <= 0 {
		return len(p), nil
	}
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = w.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = w.buf.Write(p)
	return len(p), nil
}

func (w *boundedCaptureWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func effectiveOutputCaptureLimit(outputMode string, maxBytes int) int {
	if outputMode == "none" {
		return 0
	}
	if maxBytes > 0 {
		return maxBytes
	}
	return defaultOutputCaptureBytes
}

func runCommandWithBoundedOutput(cmd *exec.Cmd, captureLimit int) (string, int, error) {
	var writer io.Writer = io.Discard
	var capture *boundedCaptureWriter
	if captureLimit > 0 {
		capture = newBoundedCaptureWriter(captureLimit)
		writer = capture
	}
	cmd.Stdout = writer
	cmd.Stderr = writer

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return "", 0, err
		}
	}
	if capture == nil {
		return "", exitCode, nil
	}
	return capture.String(), exitCode, nil
}
