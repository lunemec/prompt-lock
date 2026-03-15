package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type executeReq struct {
	LeaseToken         string   `json:"lease_token"`
	Intent             string   `json:"intent,omitempty"`
	Command            []string `json:"command"`
	Secrets            []string `json:"secrets"`
	CommandFingerprint string   `json:"command_fingerprint"`
	WorkdirFingerprint string   `json:"workdir_fingerprint"`
	TimeoutSec         int      `json:"timeout_sec"`
}

func (s *server) handleExecute(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireDurabilityReady(w) {
		return
	}
	var req executeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	if len(req.Command) == 0 {
		writeMappedError(w, ErrBadRequest, "command is required")
		return
	}
	if len(req.Secrets) == 0 {
		writeMappedError(w, ErrBadRequest, "secrets are required")
		return
	}
	at, aid := actorFromRequest(r)
	if !s.authEnabled {
		aid = ""
	}
	result, err := s.executeUseCase.Execute(r.Context(), app.ExecuteWithLeaseInput{
		ActorType:           at,
		ActorID:             aid,
		LeaseToken:          req.LeaseToken,
		Intent:              req.Intent,
		Command:             req.Command,
		Secrets:             req.Secrets,
		CommandFingerprint:  req.CommandFingerprint,
		WorkdirFingerprint:  req.WorkdirFingerprint,
		RequestedTimeoutSec: req.TimeoutSec,
	})
	if err != nil {
		switch {
		case errors.Is(err, app.ErrCommandExecutionFailed):
			writeMappedError(w, ErrInternal, err.Error())
		case errors.Is(err, app.ErrAuditUnavailable), errors.Is(err, ErrDurabilityClosed), errors.Is(err, app.ErrSecretBackendUnavailable), errors.Is(err, ports.ErrStoreUnavailable), errors.Is(err, app.ErrRequestNotOwned), errors.Is(err, app.ErrLeaseNotOwned):
			kind, msg := stateStoreAccessError(err)
			writeMappedError(w, kind, msg)
		default:
			writeMappedError(w, ErrForbidden, withPolicyHint(err.Error()))
		}
		return
	}
	resp := map[string]any{"exit_code": result.ExitCode, "stdout_stderr": result.StdoutStderr}
	if result.AuditWarning != "" {
		resp["audit_warning"] = result.AuditWarning
	}
	writeJSON(w, resp)
}
