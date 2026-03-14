package main

import (
	"path/filepath"
	"testing"
)

func TestAcquireFileLockRejectsConcurrentWriters(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "state.json.lock")

	lockA, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	t.Cleanup(func() { _ = lockA.Close() })

	lockB, err := acquireFileLock(lockPath)
	if err == nil {
		_ = lockB.Close()
		t.Fatalf("expected concurrent lock acquisition failure")
	}
}

func TestAcquireFileLockAllowsReacquireAfterRelease(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "state.json.lock")

	lockA, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	if err := lockA.Close(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}

	lockB, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquire second lock: %v", err)
	}
	if err := lockB.Close(); err != nil {
		t.Fatalf("release second lock: %v", err)
	}
}
