package main

import (
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
			rb, rs, rg := s.authStore.CleanupExpired(s.now())
			if rb+rs+rg > 0 {
				s.persistAuthStore()
				_ = s.svc.Audit.Write(ports.AuditEvent{
					Event:     "auth_cleanup",
					Timestamp: s.now(),
					ActorType: "system",
					ActorID:   "promptlockd",
					Metadata: map[string]string{
						"removed_bootstrap": itoa(uint64(rb)),
						"removed_sessions":  itoa(uint64(rs)),
						"revoked_grants":    itoa(uint64(rg)),
					},
				})
			}
		}
	}()
}
