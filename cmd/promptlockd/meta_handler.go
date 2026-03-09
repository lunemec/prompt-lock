package main

import "net/http"

func (s *server) handleMetaCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, map[string]any{
		"auth_enabled":                     s.authEnabled,
		"allow_plaintext_secret_return":    s.authCfg.AllowPlaintextSecretReturn,
		"insecure_dev_mode":                s.insecureDevMode || (!s.authEnabled && s.authCfg.AllowPlaintextSecretReturn),
		"transport_unix_socket_configured": s.unixSocketConfigured,
	})
}
