package main

import (
	"errors"
	"strings"
)

var errMissingAuthStorePersister = errors.New("auth store persister not configured")

func (s *server) persistAuthStore() error {
	if !s.authEnabled {
		return nil
	}
	path := strings.TrimSpace(s.authStoreFile)
	if path == "" {
		return nil
	}
	persister := s.authStorePersister
	if persister == nil && s.authStore != nil {
		persister = s.authStore
	}
	if persister == nil {
		return s.closeDurabilityGate("auth_store", errMissingAuthStorePersister)
	}
	var err error
	if len(s.authStoreKey) > 0 {
		err = persister.SaveToFileEncrypted(path, s.authStoreKey)
	} else {
		err = persister.SaveToFile(path)
	}
	return s.closeDurabilityGate("auth_store", err)
}
