package main

import (
	"fmt"
	"time"

	"github.com/lunemec/promptlock/internal/core/ports"
)

func startAuthCleanupLoop(s *server) {
	interval := time.Duration(s.authCfg.CleanupIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			_ = runAuthCleanupPass(s)
		}
	}()
}

func runAuthCleanupPass(s *server) error {
	s.authLifecycleMu.Lock()
	defer s.authLifecycleMu.Unlock()

	snapshot := s.authStore.Snapshot()
	rb, rs, rg := s.authStore.CleanupExpired(s.now())
	if rb+rs+rg == 0 {
		return nil
	}
	if err := s.persistAuthStore(); err != nil {
		s.authStore.Restore(snapshot)
		if rollbackErr := s.persistAuthStore(); rollbackErr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
		}
		return err
	}
	if err := s.auditCritical(ports.AuditEvent{
		Event:     "auth_cleanup",
		Timestamp: s.now(),
		ActorType: "system",
		ActorID:   "promptlockd",
		Metadata: map[string]string{
			"removed_bootstrap": itoa(uint64(rb)),
			"removed_sessions":  itoa(uint64(rs)),
			"revoked_grants":    itoa(uint64(rg)),
		},
	}); err != nil {
		s.authStore.Restore(snapshot)
		if rollbackErr := s.persistAuthStore(); rollbackErr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
		}
		return err
	}
	return nil
}
