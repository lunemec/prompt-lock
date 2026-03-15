package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func TestCloseDurabilityGateSanitizesUnavailableReasonInAuditMetadata(t *testing.T) {
	audit := &auditCapture{}
	s := &server{
		svc: app.Service{Audit: audit},
	}

	githubTokenPrefix := "gh" + "p_"
	err := ports.WrapStoreUnavailable(errors.New("external state backend returned status 500: token=" + githubTokenPrefix + "super_secret body={\"echo\":\"raw-secret\"}"))
	if got := s.closeDurabilityGate("request_lease_state", err); got == nil {
		t.Fatalf("expected durability close error")
	}

	ev, ok := findDurabilityAuditEvent(audit.events, "durability_persist_failed")
	if !ok {
		t.Fatalf("expected durability_persist_failed event, got %#v", audit.events)
	}
	if got, want := ev.Metadata["reason"], "store_unavailable_upstream_5xx"; got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
	if strings.Contains(ev.Metadata["reason"], githubTokenPrefix) || strings.Contains(ev.Metadata["reason"], "raw-secret") {
		t.Fatalf("expected sanitized audit reason, got %#v", ev.Metadata)
	}
}

func TestCloseDurabilityGateUsesStableReasonCodeAcrossRepeatedFailures(t *testing.T) {
	audit := &auditCapture{}
	s := &server{
		svc: app.Service{Audit: audit},
	}

	githubTokenPrefix := "gh" + "p_"
	errA := ports.WrapStoreUnavailable(errors.New("external state backend returned status 500: token=" + githubTokenPrefix + "first"))
	errB := ports.WrapStoreUnavailable(errors.New("external state backend returned status 500: token=" + githubTokenPrefix + "second"))

	_ = s.closeDurabilityGate("request_lease_state", errA)
	_ = s.closeDurabilityGate("request_lease_state", errB)

	reasons := durabilityEventReasons(audit.events, "durability_persist_failed")
	if len(reasons) != 2 {
		t.Fatalf("expected 2 durability_persist_failed reasons, got %#v", reasons)
	}
	if reasons[0] != reasons[1] {
		t.Fatalf("expected stable reason codes, got %#v", reasons)
	}
	if reasons[0] != "store_unavailable_upstream_5xx" {
		t.Fatalf("unexpected reason code %q", reasons[0])
	}
}

func findDurabilityAuditEvent(events []ports.AuditEvent, name string) (ports.AuditEvent, bool) {
	for _, ev := range events {
		if ev.Event == name {
			return ev, true
		}
	}
	return ports.AuditEvent{}, false
}

func durabilityEventReasons(events []ports.AuditEvent, name string) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.Event == name {
			out = append(out, ev.Metadata["reason"])
		}
	}
	return out
}
