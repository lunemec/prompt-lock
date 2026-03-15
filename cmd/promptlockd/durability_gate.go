package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/core/ports"
)

var ErrDurabilityClosed = errors.New("durability_closed")

const durabilityUnavailableMessage = "durability persistence unavailable; broker closed for safety"

func (s *server) nowUTC() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *server) durabilityError() error {
	s.durabilityMu.RLock()
	closed := s.durabilityClosed
	reason := s.durabilityReason
	s.durabilityMu.RUnlock()
	if !closed {
		return nil
	}
	if strings.TrimSpace(reason) == "" {
		reason = "persistence failure"
	}
	return fmt.Errorf("%w: %s", ErrDurabilityClosed, reason)
}

func (s *server) closeDurabilityGate(component string, err error) error {
	if err == nil {
		return nil
	}
	reason := fmt.Sprintf("%s persistence failure: %v", component, err)
	auditReason := durabilityAuditReason(err)
	log.Printf("ERROR: %s", reason)

	firstClose := false
	s.durabilityMu.Lock()
	if !s.durabilityClosed {
		s.durabilityClosed = true
		s.durabilityReason = reason
		firstClose = true
	}
	s.durabilityMu.Unlock()

	if s.svc.Audit != nil && component != "audit" {
		event := ports.AuditEvent{
			Event:     "durability_persist_failed",
			Timestamp: s.nowUTC(),
			ActorType: "system",
			ActorID:   "promptlockd",
			Metadata:  map[string]string{"component": component, "reason": auditReason},
		}
		_ = s.svc.Audit.Write(event)
		if firstClose {
			_ = s.svc.Audit.Write(ports.AuditEvent{
				Event:     "durability_gate_closed",
				Timestamp: s.nowUTC(),
				ActorType: "system",
				ActorID:   "promptlockd",
				Metadata:  map[string]string{"component": component, "reason": auditReason},
			})
		}
	}
	return fmt.Errorf("%w: %s", ErrDurabilityClosed, reason)
}

func durabilityAuditReason(err error) string {
	if err == nil {
		return "persistence_error"
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if errors.Is(err, ports.ErrStoreUnavailable) {
		switch {
		case strings.Contains(msg, "status 401"):
			return "store_unavailable_http_401"
		case strings.Contains(msg, "status 403"):
			return "store_unavailable_http_403"
		case strings.Contains(msg, "status 404"), strings.Contains(msg, "not found"):
			return "store_unavailable_not_found"
		case strings.Contains(msg, "status 408"), strings.Contains(msg, "status 429"), strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
			return "store_unavailable_timeout"
		case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"), strings.Contains(msg, "tls"), strings.Contains(msg, "x509"), strings.Contains(msg, "eof"):
			return "store_unavailable_transport"
		case strings.Contains(msg, "status 5"):
			return "store_unavailable_upstream_5xx"
		case strings.Contains(msg, "status 4"):
			return "store_unavailable_upstream_4xx"
		default:
			return "store_unavailable"
		}
	}
	switch {
	case strings.Contains(msg, "permission denied"):
		return "permission_denied"
	case strings.Contains(msg, "disk full"), strings.Contains(msg, "no space left"):
		return "no_space"
	case strings.Contains(msg, "read-only file system"):
		return "read_only"
	case strings.Contains(msg, "sync parent dir"), strings.Contains(msg, "fsync"):
		return "fsync_failed"
	case strings.Contains(msg, "rename"):
		return "rename_failed"
	default:
		return "persistence_error"
	}
}

func (s *server) requireDurabilityReady(w http.ResponseWriter) bool {
	if err := s.durabilityError(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return false
	}
	return true
}

func (s *server) auditCritical(event ports.AuditEvent) error {
	if s == nil || s.svc.Audit == nil {
		return nil
	}
	if err := s.svc.Audit.Write(event); err != nil {
		if handled := s.closeDurabilityGate("audit", err); handled != nil {
			return handled
		}
		return ErrServiceUnavailable
	}
	return nil
}
