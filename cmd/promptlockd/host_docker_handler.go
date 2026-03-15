package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/lunemec/promptlock/internal/app"
)

type hostDockerReq struct {
	Command []string `json:"command"`
}

func (s *server) handleHostDockerExecute(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireOperator(w, r)
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
	var req hostDockerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	at, aid := actorFromRequest(r)
	result, err := s.hostDockerUseCase.Execute(r.Context(), app.HostDockerExecuteInput{
		ActorType: at,
		ActorID:   aid,
		Command:   req.Command,
	})
	if err != nil {
		switch {
		case errors.Is(err, app.ErrCommandExecutionFailed):
			writeMappedError(w, ErrInternal, err.Error())
		case errors.Is(err, app.ErrAuditUnavailable), errors.Is(err, ErrDurabilityClosed):
			writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
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

func (s *server) validateHostDockerCommand(cmd []string) error {
	return s.controlPolicy().ValidateHostDockerCommand(cmd)
}
