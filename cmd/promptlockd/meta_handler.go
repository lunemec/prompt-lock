package main

import "net/http"

func (s *server) handleMetaCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	writeJSON(w, map[string]any{
		"auth_enabled":                     s.authEnabled,
		"allow_plaintext_secret_return":    s.authCfg.AllowPlaintextSecretReturn,
		"transport_unix_socket_configured": s.unixSocketConfigured,
	})
}
