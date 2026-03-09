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
	log.Printf("ERROR: %s", reason)

	firstClose := false
	s.durabilityMu.Lock()
	if !s.durabilityClosed {
		s.durabilityClosed = true
		s.durabilityReason = reason
		firstClose = true
	}
	s.durabilityMu.Unlock()

	if s.svc.Audit != nil {
		event := ports.AuditEvent{
			Event:     "durability_persist_failed",
			Timestamp: s.nowUTC(),
			ActorType: "system",
			ActorID:   "promptlockd",
			Metadata:  map[string]string{"component": component, "reason": err.Error()},
		}
		_ = s.svc.Audit.Write(event)
		if firstClose {
			_ = s.svc.Audit.Write(ports.AuditEvent{
				Event:     "durability_gate_closed",
				Timestamp: s.nowUTC(),
				ActorType: "system",
				ActorID:   "promptlockd",
				Metadata:  map[string]string{"component": component, "reason": err.Error()},
			})
		}
	}
	return fmt.Errorf("%w: %s", ErrDurabilityClosed, reason)
}

func (s *server) requireDurabilityReady(w http.ResponseWriter) bool {
	if err := s.durabilityError(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return false
	}
	return true
}
