package main

import (
	"strings"

	"github.com/lunemec/promptlock/internal/core/ports"
)

func (s *server) persistRequestLeaseState() error {
	path := strings.TrimSpace(s.stateStoreFile)
	if path == "" || s.stateStorePersister == nil {
		return nil
	}
	err := s.stateStorePersister.SaveStateToFile(path)
	return s.closeDurabilityGate("request_lease_state", err)
}

type requestLeaseStateCommitter struct {
	server *server
}

var _ ports.RequestLeaseStateCommitter = requestLeaseStateCommitter{}

func (c requestLeaseStateCommitter) CommitRequestLeaseState() error {
	if c.server == nil {
		return nil
	}
	return c.server.persistRequestLeaseState()
}

func (s *server) ensureRequestLeaseStateCommitter() {
	if s == nil || s.svc.RequestLeaseStateCommitter != nil {
		return
	}
	s.svc.RequestLeaseStateCommitter = requestLeaseStateCommitter{server: s}
}
