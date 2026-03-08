package main

import (
	"log"
	"strings"
)

func (s *server) persistAuthStore() {
	if !s.authEnabled {
		return
	}
	path := strings.TrimSpace(s.authStoreFile)
	if path == "" {
		return
	}
	if err := s.authStore.SaveToFile(path); err != nil {
		log.Printf("WARN: failed to persist auth store: %v", err)
	}
}
