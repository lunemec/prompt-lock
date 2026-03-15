package app

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type auditBuf struct{ events []ports.AuditEvent }
type failingAuditBuf struct{}
type scriptedAuditBuf struct {
	events []ports.AuditEvent
	failAt int
	calls  int
}
type concurrentApproveProbeStore struct {
	*memory.Store
	waitForSecondGet time.Duration
	firstGetStarted  chan struct{}
	secondGetSeen    chan struct{}
	firstGetOnce     sync.Once
	secondGetOnce    sync.Once
	mu               sync.Mutex
	getCalls         int
}

type failingSecretStore struct{}
type sensitiveErrorSecretStore struct{}
type countingSecretStore struct {
	calls int
	value string
}
type countingEnvPathSecretStore struct {
	resolveCalls int
	resolved     map[string]string
	canonical    string
}

type failingLeaseStore struct{ err error }
type failingRequestLeaseStateCommitter struct{ err error }
type persistThenFailRequestLeaseStateCommitter struct {
	source *memory.Store
	path   string
	failAt int
	calls  int
}

func (failingSecretStore) GetSecret(string) (string, error) { return "", errors.New("backend timeout") }
func (sensitiveErrorSecretStore) GetSecret(string) (string, error) {
	return "", errors.New("external backend returned status 500: raw-secret-value")
}
func (s *countingSecretStore) GetSecret(string) (string, error) {
	s.calls++
	return s.value, nil
}
func (s *countingEnvPathSecretStore) Canonicalize(string) (string, error) {
	if s.canonical != "" {
		return s.canonical, nil
	}
	return "/workspace/.env", nil
}
func (s *countingEnvPathSecretStore) Resolve(string, []string) (map[string]string, string, error) {
	s.resolveCalls++
	return s.resolved, s.canonical, nil
}

func (f failingLeaseStore) SaveLease(domain.Lease) error { return f.err }
func (failingLeaseStore) DeleteLease(string) error       { return errors.New("lease not found") }
func (failingLeaseStore) GetLease(string) (domain.Lease, error) {
	return domain.Lease{}, errors.New("lease not found")
}
func (failingLeaseStore) GetLeaseByRequestID(string) (domain.Lease, error) {
	return domain.Lease{}, errors.New("lease not found")
}

func (f failingRequestLeaseStateCommitter) CommitRequestLeaseState() error { return f.err }

func (c *persistThenFailRequestLeaseStateCommitter) CommitRequestLeaseState() error {
	c.calls++
	if err := c.source.SaveStateToFile(c.path); err != nil {
		return err
	}
	if c.failAt > 0 && c.calls == c.failAt {
		return errors.New("parent dir sync failed after rename")
	}
	return nil
}

func (a *auditBuf) Write(e ports.AuditEvent) error {
	a.events = append(a.events, e)
	return nil
}

func (failingAuditBuf) Write(ports.AuditEvent) error { return errors.New("audit unavailable") }

func (a *scriptedAuditBuf) Write(e ports.AuditEvent) error {
	a.calls++
	a.events = append(a.events, e)
	if a.failAt > 0 && a.calls == a.failAt {
		return errors.New("audit unavailable")
	}
	return nil
}

func (s *concurrentApproveProbeStore) GetRequest(id string) (domain.LeaseRequest, error) {
	req, err := s.Store.GetRequest(id)
	if err != nil {
		return domain.LeaseRequest{}, err
	}

	s.mu.Lock()
	s.getCalls++
	call := s.getCalls
	s.mu.Unlock()

	switch call {
	case 1:
		s.firstGetOnce.Do(func() { close(s.firstGetStarted) })
		select {
		case <-s.secondGetSeen:
		case <-time.After(s.waitForSecondGet):
		}
	case 2:
		s.secondGetOnce.Do(func() { close(s.secondGetSeen) })
	}

	return req, nil
}

func TestLeaseFlow(t *testing.T) {
	store := memory.NewStore()
	store.SetSecret("github_token", "x")
	a := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	seq := 0
	svc := Service{
		Policy:   domain.DefaultPolicy(),
		Requests: store,
		Leases:   store,
		Secrets:  store,
		Audit:    a,
		Now:      func() time.Time { return now },
		NewRequestID: func() string {
			seq++
			return "req_test"
		},
		NewLeaseTok: func() string { return "lease_test" },
	}

	req, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.RequestPending {
		t.Fatalf("expected pending")
	}

	lease, err := svc.ApproveRequest(req.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Token == "" {
		t.Fatalf("expected lease token")
	}

	val, err := svc.AccessSecret(lease.Token, "github_token", "fp1", "wd1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "x" {
		t.Fatalf("unexpected secret value")
	}

	if len(a.events) < 3 {
		t.Fatalf("expected audit events")
	}

	if _, err := svc.AccessSecret(lease.Token, "github_token", "different-fp", "wd1"); err == nil {
		t.Fatalf("expected fingerprint mismatch error")
	}
	if _, err := svc.AccessSecret(lease.Token, "github_token", "fp1", "other-wd"); err == nil {
		t.Fatalf("expected workdir mismatch error")
	}
}

func TestApproveRequestRollsBackPendingStateWhenLeaseSaveFails(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_rollback",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       failingLeaseStore{err: errors.New("lease backend unavailable")},
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_unused" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	if _, err := svc.ApproveRequest("req_rollback", 5); err == nil || err.Error() != "lease backend unavailable" {
		t.Fatalf("expected lease save failure, got %v", err)
	}

	stored, err := store.GetRequest("req_rollback")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected rollback to pending, got %s", stored.Status)
	}
}

func TestRequestLeaseRollsBackWhenAuditFails(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        failingAuditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_audit_rollback" },
		NewLeaseTok:  func() string { return "lease_unused" },
	}

	if _, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1", "", ""); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	if _, err := store.GetRequest("req_audit_rollback"); err == nil {
		t.Fatalf("expected request rollback after audit failure")
	}
}

func TestApproveRequestRollsBackWhenAuditFails(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_audit_approve",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        failingAuditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_unused" },
		NewLeaseTok:  func() string { return "lease_audit_approve" },
	}

	if _, err := svc.ApproveRequest("req_audit_approve", 5); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	stored, err := store.GetRequest("req_audit_approve")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
	if _, err := store.GetLease("lease_audit_approve"); err == nil {
		t.Fatalf("expected lease rollback after audit failure")
	}
}

func TestApproveRequestCarriesEnvPathMetadataOnPrimaryAudit(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_env_audit_approve",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &scriptedAuditBuf{failAt: 2},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_unused" },
		NewLeaseTok:  func() string { return "lease_env_audit_approve" },
	}

	lease, err := svc.ApproveRequest("req_env_audit_approve", 5)
	if err != nil {
		t.Fatalf("expected approve success, got %v", err)
	}
	stored, err := store.GetRequest("req_env_audit_approve")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestApproved {
		t.Fatalf("expected request to stay approved, got %s", stored.Status)
	}
	if _, err := store.GetLease(lease.Token); err != nil {
		t.Fatalf("expected lease to stay committed, got %v", err)
	}
	foundPrimary := false
	for _, ev := range svc.Audit.(*scriptedAuditBuf).events {
		if ev.Event != "request_approved" {
			continue
		}
		foundPrimary = true
		if ev.Metadata["env_path_original"] != "./.env" {
			t.Fatalf("expected primary approval audit to carry env_path_original, got %q", ev.Metadata["env_path_original"])
		}
		if ev.Metadata["env_path_canonical"] != "/workspace/.env" {
			t.Fatalf("expected primary approval audit to carry env_path_canonical, got %q", ev.Metadata["env_path_canonical"])
		}
	}
	if !foundPrimary {
		t.Fatalf("expected request_approved audit event")
	}
}

func TestApproveRequestSerializesConcurrentMutation(t *testing.T) {
	now := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	store := &concurrentApproveProbeStore{
		Store:            memory.NewStore(),
		waitForSecondGet: 100 * time.Millisecond,
		firstGetStarted:  make(chan struct{}),
		secondGetSeen:    make(chan struct{}),
	}
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_concurrent_approve",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	leaseTokens := make(chan string, 2)
	leaseTokens <- "lease-concurrent-1"
	leaseTokens <- "lease-concurrent-2"
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		MutationLock: &sync.Mutex{},
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "unused-request-id" },
		NewLeaseTok: func() string {
			return <-leaseTokens
		},
	}

	results := make(chan error, 2)
	go func() {
		_, err := svc.ApproveRequest("req_concurrent_approve", 5)
		results <- err
	}()

	<-store.firstGetStarted
	go func() {
		_, err := svc.ApproveRequest("req_concurrent_approve", 5)
		results <- err
	}()

	var successCount int
	var notPendingCount int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successCount++
			continue
		}
		if err.Error() == "request is not pending" {
			notPendingCount++
			continue
		}
		t.Fatalf("unexpected concurrent approve error: %v", err)
	}

	if successCount != 1 {
		t.Fatalf("expected exactly one successful approval, got %d", successCount)
	}
	if notPendingCount != 1 {
		t.Fatalf("expected exactly one non-pending rejection, got %d", notPendingCount)
	}

	leases, err := store.ListLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 1 {
		t.Fatalf("expected exactly one lease after concurrent approvals, got %#v", leases)
	}
}

func TestDenyRequestRollsBackWhenAuditFails(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_audit_deny",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        failingAuditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_unused" },
		NewLeaseTok:  func() string { return "lease_unused" },
	}

	if _, err := svc.DenyRequest("req_audit_deny", "deny"); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	stored, err := store.GetRequest("req_audit_deny")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
}

func TestDenyRequestCarriesEnvPathMetadataOnPrimaryAudit(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_env_audit_deny",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &scriptedAuditBuf{failAt: 2},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_unused" },
		NewLeaseTok:  func() string { return "lease_unused" },
	}

	if _, err := svc.DenyRequest("req_env_audit_deny", "deny"); err != nil {
		t.Fatalf("expected deny success, got %v", err)
	}
	stored, err := store.GetRequest("req_env_audit_deny")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestDenied {
		t.Fatalf("expected request to stay denied, got %s", stored.Status)
	}
	foundPrimary := false
	for _, ev := range svc.Audit.(*scriptedAuditBuf).events {
		if ev.Event != "request_denied" {
			continue
		}
		foundPrimary = true
		if ev.Metadata["env_path_original"] != "./.env" {
			t.Fatalf("expected primary deny audit to carry env_path_original, got %q", ev.Metadata["env_path_original"])
		}
		if ev.Metadata["env_path_canonical"] != "/workspace/.env" {
			t.Fatalf("expected primary deny audit to carry env_path_canonical, got %q", ev.Metadata["env_path_canonical"])
		}
	}
	if !foundPrimary {
		t.Fatalf("expected request_denied audit event")
	}
}

func TestCancelRequestByAgentRollsBackWhenAuditFails(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_audit_cancel",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        failingAuditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_unused" },
		NewLeaseTok:  func() string { return "lease_unused" },
	}

	if _, err := svc.CancelRequestByAgent("req_audit_cancel", "agent1", "cancel"); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	stored, err := store.GetRequest("req_audit_cancel")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
}

func TestRequestLeaseRollsBackWhenStateCommitFails(t *testing.T) {
	store := memory.NewStore()
	audit := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: failingRequestLeaseStateCommitter{err: errors.New("disk full")},
		Secrets:                    store,
		Audit:                      audit,
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_commit_rollback" },
		NewLeaseTok:                func() string { return "lease_unused" },
	}

	if _, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1", "", ""); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	if _, err := store.GetRequest("req_commit_rollback"); err == nil {
		t.Fatalf("expected request rollback after state commit failure")
	}
	if len(audit.events) != 0 {
		t.Fatalf("expected no success audit events after state commit failure, got %d", len(audit.events))
	}
}

func TestApproveRequestRollsBackWhenStateCommitFails(t *testing.T) {
	store := memory.NewStore()
	audit := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_commit_approve",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: failingRequestLeaseStateCommitter{err: errors.New("disk full")},
		Secrets:                    store,
		Audit:                      audit,
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_unused" },
		NewLeaseTok:                func() string { return "lease_commit_approve" },
	}

	if _, err := svc.ApproveRequest("req_commit_approve", 5); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	stored, err := store.GetRequest("req_commit_approve")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
	if _, err := store.GetLease("lease_commit_approve"); err == nil {
		t.Fatalf("expected lease rollback after state commit failure")
	}
	if len(audit.events) != 0 {
		t.Fatalf("expected no success audit events after state commit failure, got %d", len(audit.events))
	}
}

func TestDenyRequestRollsBackWhenStateCommitFails(t *testing.T) {
	store := memory.NewStore()
	audit := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_commit_deny",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: failingRequestLeaseStateCommitter{err: errors.New("disk full")},
		Secrets:                    store,
		Audit:                      audit,
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_unused" },
		NewLeaseTok:                func() string { return "lease_unused" },
	}

	if _, err := svc.DenyRequest("req_commit_deny", "deny"); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	stored, err := store.GetRequest("req_commit_deny")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
	if len(audit.events) != 0 {
		t.Fatalf("expected no success audit events after state commit failure, got %d", len(audit.events))
	}
}

func TestCancelRequestByAgentRollsBackWhenStateCommitFails(t *testing.T) {
	store := memory.NewStore()
	audit := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_commit_cancel",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}

	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: failingRequestLeaseStateCommitter{err: errors.New("disk full")},
		Secrets:                    store,
		Audit:                      audit,
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_unused" },
		NewLeaseTok:                func() string { return "lease_unused" },
	}

	if _, err := svc.CancelRequestByAgent("req_commit_cancel", "agent1", "cancel"); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	stored, err := store.GetRequest("req_commit_cancel")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
	if len(audit.events) != 0 {
		t.Fatalf("expected no success audit events after state commit failure, got %d", len(audit.events))
	}
}

func TestRequestLeaseRePersistsRollbackWhenStateCommitFailsAfterDurableWrite(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state-store.json")
	committer := &persistThenFailRequestLeaseStateCommitter{source: store, path: path, failAt: 1}
	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: committer,
		Secrets:                    store,
		Audit:                      &auditBuf{},
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_commit_after_write" },
		NewLeaseTok:                func() string { return "lease_unused" },
	}

	if _, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1", "", ""); err == nil {
		t.Fatalf("expected commit failure")
	}
	if committer.calls != 2 {
		t.Fatalf("expected commit + rollback persist, got %d commits", committer.calls)
	}

	reloaded := memory.NewStore()
	if err := reloaded.LoadStateFromFile(path); err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, err := reloaded.GetRequest("req_commit_after_write"); err == nil {
		t.Fatalf("expected persisted request rollback after commit failure")
	}
}

func TestApproveRequestRePersistsRollbackWhenStateCommitFailsAfterDurableWrite(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state-store.json")
	req := domain.LeaseRequest{
		ID:                 "req_commit_approve_after_write",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}
	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	committer := &persistThenFailRequestLeaseStateCommitter{source: store, path: path, failAt: 1}
	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: committer,
		Secrets:                    store,
		Audit:                      &auditBuf{},
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_unused" },
		NewLeaseTok:                func() string { return "lease_commit_approve_after_write" },
	}

	if _, err := svc.ApproveRequest(req.ID, 5); err == nil {
		t.Fatalf("expected commit failure")
	}
	if committer.calls != 2 {
		t.Fatalf("expected commit + rollback persist, got %d commits", committer.calls)
	}

	reloaded := memory.NewStore()
	if err := reloaded.LoadStateFromFile(path); err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	stored, err := reloaded.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected persisted request rollback to pending, got %s", stored.Status)
	}
	if _, err := reloaded.GetLease("lease_commit_approve_after_write"); err == nil {
		t.Fatalf("expected persisted lease rollback after commit failure")
	}
}

func TestDenyRequestRePersistsRollbackWhenStateCommitFailsAfterDurableWrite(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state-store.json")
	req := domain.LeaseRequest{
		ID:                 "req_commit_deny_after_write",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}
	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	committer := &persistThenFailRequestLeaseStateCommitter{source: store, path: path, failAt: 1}
	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: committer,
		Secrets:                    store,
		Audit:                      &auditBuf{},
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_unused" },
		NewLeaseTok:                func() string { return "lease_unused" },
	}

	if _, err := svc.DenyRequest(req.ID, "deny"); err == nil {
		t.Fatalf("expected commit failure")
	}
	if committer.calls != 2 {
		t.Fatalf("expected commit + rollback persist, got %d commits", committer.calls)
	}

	reloaded := memory.NewStore()
	if err := reloaded.LoadStateFromFile(path); err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	stored, err := reloaded.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected persisted request rollback to pending, got %s", stored.Status)
	}
}

func TestCancelRequestByAgentRePersistsRollbackWhenStateCommitFailsAfterDurableWrite(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state-store.json")
	req := domain.LeaseRequest{
		ID:                 "req_commit_cancel_after_write",
		AgentID:            "agent1",
		TaskID:             "task1",
		Reason:             "test",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}
	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	committer := &persistThenFailRequestLeaseStateCommitter{source: store, path: path, failAt: 1}
	svc := Service{
		Policy:                     domain.DefaultPolicy(),
		Requests:                   store,
		Leases:                     store,
		RequestLeaseStateCommitter: committer,
		Secrets:                    store,
		Audit:                      &auditBuf{},
		Now:                        func() time.Time { return now },
		NewRequestID:               func() string { return "req_unused" },
		NewLeaseTok:                func() string { return "lease_unused" },
	}

	if _, err := svc.CancelRequestByAgent(req.ID, "agent1", "cancel"); err == nil {
		t.Fatalf("expected commit failure")
	}
	if committer.calls != 2 {
		t.Fatalf("expected commit + rollback persist, got %d commits", committer.calls)
	}

	reloaded := memory.NewStore()
	if err := reloaded.LoadStateFromFile(path); err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	stored, err := reloaded.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected persisted request rollback to pending, got %s", stored.Status)
	}
}

func TestAccessSecretBackendFailureIsAuditedAndDeterministic(t *testing.T) {
	store := memory.NewStore()
	a := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      failingSecretStore{},
		Audit:        a,
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_test" },
		NewLeaseTok:  func() string { return "lease_test" },
	}

	_ = store.SaveRequest(domain.LeaseRequest{ID: "req_test", AgentID: "agent1", TaskID: "task1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "lease_test", RequestID: "req_test", AgentID: "agent1", TaskID: "task1", Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", ExpiresAt: now.Add(5 * time.Minute)})

	_, err := svc.AccessSecret("lease_test", "github_token", "fp1", "wd1")
	if err == nil || err.Error() != "secret backend unavailable" {
		t.Fatalf("expected deterministic backend error, got %v", err)
	}

	found := false
	for _, ev := range a.events {
		if ev.Event == "secret_backend_error" {
			found = true
			if ev.Metadata["reason"] == "" {
				t.Fatalf("expected backend error reason metadata")
			}
		}
	}
	if !found {
		t.Fatalf("expected secret_backend_error audit event")
	}
}

func TestAccessSecretBackendFailureAuditDoesNotLeakRawBackendError(t *testing.T) {
	store := memory.NewStore()
	a := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      sensitiveErrorSecretStore{},
		Audit:        a,
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_test" },
		NewLeaseTok:  func() string { return "lease_test" },
	}

	_ = store.SaveRequest(domain.LeaseRequest{ID: "req_test", AgentID: "agent1", TaskID: "task1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "lease_test", RequestID: "req_test", AgentID: "agent1", TaskID: "task1", Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", ExpiresAt: now.Add(5 * time.Minute)})

	_, err := svc.AccessSecret("lease_test", "github_token", "fp1", "wd1")
	if err == nil || err.Error() != "secret backend unavailable" {
		t.Fatalf("expected deterministic backend error, got %v", err)
	}

	ev, ok := findAuditEvent(a.events, "secret_backend_error")
	if !ok {
		t.Fatalf("expected secret_backend_error audit event")
	}
	if strings.Contains(ev.Metadata["reason"], "raw-secret-value") {
		t.Fatalf("expected audit reason to avoid raw backend error content, got %q", ev.Metadata["reason"])
	}
	if ev.Metadata["reason"] == "" {
		t.Fatalf("expected non-empty sanitized reason")
	}
}

func TestAccessSecretDoesNotReadSecretWhenPreAccessAuditFails(t *testing.T) {
	store := memory.NewStore()
	secrets := &countingSecretStore{value: "x"}
	audit := &scriptedAuditBuf{failAt: 1}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      secrets,
		Audit:        audit,
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_test" },
		NewLeaseTok:  func() string { return "lease_test" },
	}

	_ = store.SaveRequest(domain.LeaseRequest{ID: "req_test", AgentID: "agent1", TaskID: "task1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "lease_test", RequestID: "req_test", AgentID: "agent1", TaskID: "task1", Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", ExpiresAt: now.Add(5 * time.Minute)})

	_, err := svc.AccessSecret("lease_test", "github_token", "fp1", "wd1")
	if !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure before secret read, got %v", err)
	}
	if secrets.calls != 0 {
		t.Fatalf("expected secret backend read to stay blocked, got %d calls", secrets.calls)
	}
	if len(audit.events) != 1 || audit.events[0].Event != AuditEventSecretAccessStarted {
		t.Fatalf("expected only %s before failure, got %+v", AuditEventSecretAccessStarted, audit.events)
	}
}

func TestResolveExecutionSecretsDoesNotReadEnvPathWhenPreAccessAuditFails(t *testing.T) {
	store := memory.NewStore()
	envStore := &countingEnvPathSecretStore{
		resolved:  map[string]string{"github_token": "dotenv-value"},
		canonical: "/workspace/.env",
	}
	audit := &scriptedAuditBuf{failAt: 1}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:         domain.DefaultPolicy(),
		Requests:       store,
		Leases:         store,
		Secrets:        &countingSecretStore{value: "primary"},
		EnvPathSecrets: envStore,
		Audit:          audit,
		Now:            func() time.Time { return now },
		NewRequestID:   func() string { return "req_test" },
		NewLeaseTok:    func() string { return "lease_test" },
	}

	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_test",
		AgentID:            "agent1",
		TaskID:             "task1",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestApproved,
		CreatedAt:          now,
	})
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_test",
		RequestID:          "req_test",
		AgentID:            "agent1",
		TaskID:             "task1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(5 * time.Minute),
	})

	_, err := svc.ResolveExecutionSecrets("lease_test", []string{"github_token"}, "fp1", "wd1")
	if !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected audit failure before env-path read, got %v", err)
	}
	if envStore.resolveCalls != 0 {
		t.Fatalf("expected env-path secret read to stay blocked, got %d calls", envStore.resolveCalls)
	}
	if len(audit.events) != 1 || audit.events[0].Event != AuditEventSecretAccessStarted {
		t.Fatalf("expected only %s before failure, got %+v", AuditEventSecretAccessStarted, audit.events)
	}
}

func TestRequestLeaseWithPolicyReusesEquivalentActiveLease(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_active",
		RequestID:          "req_existing",
		AgentID:            "agent1",
		TaskID:             "task-old",
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(10 * time.Minute),
	})

	svc := Service{
		Policy:   domain.DefaultPolicy(),
		Requests: store,
		Leases:   store,
		Secrets:  store,
		Audit:    &auditBuf{},
		Now:      func() time.Time { return now },
		NewRequestID: func() string {
			return "req_new"
		},
		NewLeaseTok: func() string { return "lease_new" },
	}

	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "test", 5, []string{" npm_token ", "github_token", "github_token"}, "fp1", "wd1", "", "")
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if !result.Reused {
		t.Fatalf("expected equivalent active lease to be reused")
	}
	if result.Lease.Token != "lease_active" {
		t.Fatalf("expected reused lease token lease_active, got %q", result.Lease.Token)
	}
	if result.Request.ID != "" {
		t.Fatalf("expected no new request when reusing active lease, got %q", result.Request.ID)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending requests when lease is reused, got %d", len(pending))
	}
}

func TestRequestLeaseWithPolicyCreatesPendingWhenNoActiveEquivalentLease(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:   domain.DefaultPolicy(),
		Requests: store,
		Leases:   store,
		Secrets:  store,
		Audit:    &auditBuf{},
		Now:      func() time.Time { return now },
		NewRequestID: func() string {
			return "req_new"
		},
		NewLeaseTok: func() string { return "lease_new" },
	}

	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1", "", "")
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if result.Reused {
		t.Fatalf("expected new pending request when no reusable lease exists")
	}
	if result.Request.ID != "req_new" {
		t.Fatalf("expected created request id req_new, got %q", result.Request.ID)
	}
	if result.Request.Status != domain.RequestPending {
		t.Fatalf("expected pending request status, got %q", result.Request.Status)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "req_new" {
		t.Fatalf("expected created request to be pending, got %#v", pending)
	}
}

func TestRequestLeaseWithPolicyHonorsDisabledActiveLeaseReuse(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_active",
		RequestID:          "req_existing",
		AgentID:            "agent1",
		TaskID:             "task-old",
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(10 * time.Minute),
	})

	svc := Service{
		Policy:        domain.DefaultPolicy(),
		RequestPolicy: RequestPolicy{IdenticalRequestCooldown: 60 * time.Second, MaxPendingPerAgent: 2, EnableActiveLeaseReuse: false},
		Requests:      store,
		Leases:        store,
		Secrets:       store,
		Audit:         &auditBuf{},
		Now:           func() time.Time { return now },
		NewRequestID:  func() string { return "req_new" },
		NewLeaseTok:   func() string { return "lease_new" },
	}

	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "test", 5, []string{"github_token", "npm_token"}, "fp1", "wd1", "", "")
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if result.Reused {
		t.Fatalf("expected disabled active lease reuse to force a new request")
	}
	if result.Request.ID != "req_new" {
		t.Fatalf("expected created request id req_new, got %q", result.Request.ID)
	}
}

func TestRequestLeaseWithPolicyHonorsCustomPendingCap(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-5 * time.Second),
	})

	svc := Service{
		Policy:        domain.DefaultPolicy(),
		RequestPolicy: RequestPolicy{IdenticalRequestCooldown: 60 * time.Second, MaxPendingPerAgent: 1, EnableActiveLeaseReuse: true},
		Requests:      store,
		Leases:        store,
		Secrets:       store,
		Audit:         &auditBuf{},
		Now:           func() time.Time { return now },
		NewRequestID:  func() string { return "req_new" },
		NewLeaseTok:   func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task2", "second", 5, []string{"npm_token"}, "fp2", "wd2", "", "")
	if err == nil {
		t.Fatalf("expected custom pending cap to throttle")
	}
	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected request throttle error, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonPendingCap {
		t.Fatalf("expected pending-cap throttle, got %q", throttleErr.Reason)
	}
}

func TestRequestLeaseWithPolicyThrottlesWhenPendingCapReached(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-20 * time.Second),
	})
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_2",
		AgentID:            "agent1",
		TaskID:             "task_2",
		Reason:             "second",
		TTLMinutes:         5,
		Secrets:            []string{"npm_token"},
		CommandFingerprint: "fp2",
		WorkdirFingerprint: "wd2",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-10 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task3", "third", 5, []string{"slack_token"}, "fp3", "wd3", "", "")
	if err == nil {
		t.Fatalf("expected pending-cap throttle error")
	}

	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonPendingCap {
		t.Fatalf("expected pending-cap throttle reason, got %q", throttleErr.Reason)
	}
	if throttleErr.RetryAfter != 60*time.Second {
		t.Fatalf("expected retry-after 60s for pending-cap, got %s", throttleErr.RetryAfter)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected pending queue unchanged on throttle, got %d", len(pending))
	}
}

func TestOwnershipChecksDenyCrossAgentReadsAndUse(t *testing.T) {
	store := memory.NewStore()
	store.SetSecret("github_token", "x")
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_test" },
		NewLeaseTok:  func() string { return "lease_test" },
	}

	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_test",
		AgentID:            "agent-a",
		TaskID:             "task-a",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestApproved,
		CreatedAt:          now,
	})
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_test",
		RequestID:          "req_test",
		AgentID:            "agent-a",
		TaskID:             "task-a",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(5 * time.Minute),
	})

	if _, err := svc.RequestStatusByAgent("req_test", "agent-b"); !errors.Is(err, ErrRequestNotOwned) {
		t.Fatalf("expected ErrRequestNotOwned from request status, got %v", err)
	}
	if _, err := svc.LeaseByRequestForAgent("req_test", "agent-b"); !errors.Is(err, ErrRequestNotOwned) {
		t.Fatalf("expected ErrRequestNotOwned from lease by request, got %v", err)
	}
	if _, err := svc.AccessSecretByAgent("agent-b", "lease_test", "github_token", "fp1", "wd1"); !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("expected ErrLeaseNotOwned from access secret, got %v", err)
	}
	if _, err := svc.ResolveExecutionSecretsByAgent("agent-b", "lease_test", []string{"github_token"}, "fp1", "wd1"); !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("expected ErrLeaseNotOwned from resolve execution secrets, got %v", err)
	}
}

func TestRequestLeaseWithPolicyThrottlesEquivalentRequestWithinCooldown(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-15 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task2", "repeat", 5, []string{" npm_token ", "github_token"}, "fp1", "wd1", "", "")
	if err == nil {
		t.Fatalf("expected cooldown throttle error")
	}

	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonCooldown {
		t.Fatalf("expected cooldown throttle reason, got %q", throttleErr.Reason)
	}
	if throttleErr.RetryAfter != 45*time.Second {
		t.Fatalf("expected retry-after 45s for cooldown, got %s", throttleErr.RetryAfter)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected pending queue unchanged on cooldown throttle, got %d", len(pending))
	}
}

func TestRequestLeaseWithPolicyChecksPendingCapBeforeCooldown(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-10 * time.Second),
	})
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_2",
		AgentID:            "agent1",
		TaskID:             "task_2",
		Reason:             "second",
		TTLMinutes:         5,
		Secrets:            []string{"npm_token"},
		CommandFingerprint: "fp2",
		WorkdirFingerprint: "wd2",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-40 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task2", "repeat", 5, []string{"github_token"}, "fp1", "wd1", "", "")
	if err == nil {
		t.Fatalf("expected pending-cap throttle error")
	}

	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonPendingCap {
		t.Fatalf("expected pending-cap to win when both checks match, got %q", throttleErr.Reason)
	}
}

func TestRequestLeaseWithPolicyDoesNotReuseActiveLeaseWhenEnvPathProvided(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_active",
		RequestID:          "req_existing",
		AgentID:            "agent1",
		TaskID:             "task-old",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(10 * time.Minute),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new_env" },
		NewLeaseTok:  func() string { return "lease_new_env" },
	}

	result, err := svc.RequestLeaseWithPolicy(
		"agent1",
		"task-new",
		"requires env path confirmation",
		5,
		[]string{"github_token"},
		"fp1",
		"wd1",
		"./.env",
		"/workspace/project/.env",
	)
	if err != nil {
		t.Fatalf("request lease with env path: %v", err)
	}
	if result.Reused {
		t.Fatalf("expected env-path request to require fresh approval instead of active-lease reuse")
	}
	if result.Request.ID != "req_new_env" {
		t.Fatalf("expected new pending request, got %#v", result.Request)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "req_new_env" {
		t.Fatalf("expected one new pending request, got %#v", pending)
	}
}
