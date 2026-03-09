package memory

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestSaveLoadStateRoundTripRequestsAndLeases(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "memory-state.json")

	s1 := NewStore()
	reqPending := domain.LeaseRequest{
		ID:                 "req-pending",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Reason:             "pending request",
		TTLMinutes:         10,
		Secrets:            []string{"secret-a", "secret-b"},
		CommandFingerprint: "cmd-fp-1",
		WorkdirFingerprint: "wd-fp-1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}
	reqApproved := domain.LeaseRequest{
		ID:                 "req-approved",
		AgentID:            "agent-2",
		TaskID:             "task-2",
		Reason:             "approved request",
		TTLMinutes:         15,
		Secrets:            []string{"secret-c"},
		CommandFingerprint: "cmd-fp-2",
		WorkdirFingerprint: "wd-fp-2",
		Status:             domain.RequestApproved,
		CreatedAt:          now.Add(1 * time.Minute),
	}
	lease := domain.Lease{
		Token:              "lease-1",
		RequestID:          reqApproved.ID,
		AgentID:            reqApproved.AgentID,
		TaskID:             reqApproved.TaskID,
		Secrets:            []string{"secret-c"},
		CommandFingerprint: reqApproved.CommandFingerprint,
		WorkdirFingerprint: reqApproved.WorkdirFingerprint,
		ExpiresAt:          now.Add(15 * time.Minute),
	}
	if err := s1.SaveRequest(reqPending); err != nil {
		t.Fatalf("save pending request: %v", err)
	}
	if err := s1.SaveRequest(reqApproved); err != nil {
		t.Fatalf("save approved request: %v", err)
	}
	if err := s1.SaveLease(lease); err != nil {
		t.Fatalf("save lease: %v", err)
	}

	if err := s1.SaveStateToFile(path); err != nil {
		t.Fatalf("save state to file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file has insecure permissions: %o", info.Mode().Perm())
	}

	s2 := NewStore()
	if err := s2.LoadStateFromFile(path); err != nil {
		t.Fatalf("load state from file: %v", err)
	}

	gotPending, err := s2.GetRequest(reqPending.ID)
	if err != nil {
		t.Fatalf("get pending request after load: %v", err)
	}
	if !reflect.DeepEqual(reqPending, gotPending) {
		t.Fatalf("pending request mismatch after load\nwant: %#v\ngot:  %#v", reqPending, gotPending)
	}

	gotApproved, err := s2.GetRequest(reqApproved.ID)
	if err != nil {
		t.Fatalf("get approved request after load: %v", err)
	}
	if !reflect.DeepEqual(reqApproved, gotApproved) {
		t.Fatalf("approved request mismatch after load\nwant: %#v\ngot:  %#v", reqApproved, gotApproved)
	}

	gotLease, err := s2.GetLease(lease.Token)
	if err != nil {
		t.Fatalf("get lease after load: %v", err)
	}
	if !reflect.DeepEqual(lease, gotLease) {
		t.Fatalf("lease mismatch after load\nwant: %#v\ngot:  %#v", lease, gotLease)
	}
}

func TestLoadStateFromFileMissingAndEmptyDoesNotFail(t *testing.T) {
	s := NewStore()
	baseDir := t.TempDir()

	missingPath := filepath.Join(baseDir, "missing-state.json")
	if err := s.LoadStateFromFile(missingPath); err != nil {
		t.Fatalf("load missing file should not fail: %v", err)
	}

	emptyPath := filepath.Join(baseDir, "empty-state.json")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	if err := s.LoadStateFromFile(emptyPath); err != nil {
		t.Fatalf("load empty file should not fail: %v", err)
	}
}

func TestSaveLoadStatePreservesRequestStatuses(t *testing.T) {
	now := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "status-state.json")

	s1 := NewStore()
	requests := []domain.LeaseRequest{
		{
			ID:                 "req-approved",
			AgentID:            "agent-a",
			TaskID:             "task-a",
			TTLMinutes:         5,
			Secrets:            []string{"s1"},
			CommandFingerprint: "cmd-a",
			WorkdirFingerprint: "wd-a",
			Status:             domain.RequestApproved,
			CreatedAt:          now,
		},
		{
			ID:                 "req-denied",
			AgentID:            "agent-d",
			TaskID:             "task-d",
			TTLMinutes:         5,
			Secrets:            []string{"s2"},
			CommandFingerprint: "cmd-d",
			WorkdirFingerprint: "wd-d",
			Status:             domain.RequestDenied,
			CreatedAt:          now.Add(1 * time.Minute),
		},
		{
			ID:                 "req-revoked",
			AgentID:            "agent-r",
			TaskID:             "task-r",
			TTLMinutes:         5,
			Secrets:            []string{"s3"},
			CommandFingerprint: "cmd-r",
			WorkdirFingerprint: "wd-r",
			Status:             domain.RequestStatus("revoked"),
			CreatedAt:          now.Add(2 * time.Minute),
		},
	}

	for _, req := range requests {
		if err := s1.SaveRequest(req); err != nil {
			t.Fatalf("save request %s: %v", req.ID, err)
		}
	}
	if err := s1.SaveStateToFile(path); err != nil {
		t.Fatalf("save state to file: %v", err)
	}

	s2 := NewStore()
	if err := s2.LoadStateFromFile(path); err != nil {
		t.Fatalf("load state from file: %v", err)
	}

	for _, want := range requests {
		got, err := s2.GetRequest(want.ID)
		if err != nil {
			t.Fatalf("get request %s after load: %v", want.ID, err)
		}
		if got.Status != want.Status {
			t.Fatalf("status mismatch for %s: want=%q got=%q", want.ID, want.Status, got.Status)
		}
	}
}
