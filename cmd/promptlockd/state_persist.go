package main

import (
	"strings"
)

func (s *server) persistRequestLeaseState() error {
	path := strings.TrimSpace(s.stateStoreFile)
	if path == "" || s.stateStorePersister == nil {
		return nil
	}
	err := s.stateStorePersister.SaveStateToFile(path)
	return s.closeDurabilityGate("request_lease_state", err)
}
