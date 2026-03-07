package app

import (
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type auditBuf struct{ events []ports.AuditEvent }

func (a *auditBuf) Write(e ports.AuditEvent) error {
	a.events = append(a.events, e)
	return nil
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

	req, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"})
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

	val, err := svc.AccessSecret(lease.Token, "github_token")
	if err != nil {
		t.Fatal(err)
	}
	if val != "x" {
		t.Fatalf("unexpected secret value")
	}

	if len(a.events) < 3 {
		t.Fatalf("expected audit events")
	}
}
