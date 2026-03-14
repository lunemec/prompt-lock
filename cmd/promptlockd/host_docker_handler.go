package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/core/ports"
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
	var req hostDockerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	if err := s.controlPolicy().ValidateHostDockerCommand(req.Command); err != nil {
		writeMappedError(w, ErrForbidden, withPolicyHint(err.Error()))
		return
	}
	timeout := s.hostOpsPolicy.DockerTimeoutSec
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	resolvedCommand, err := s.controlPolicy().ResolveHostDockerCommand(req.Command)
	if err != nil {
		writeMappedError(w, ErrForbidden, withPolicyHint(err.Error()))
		return
	}
	cmd := exec.CommandContext(ctx, resolvedCommand.Path, resolvedCommand.Args...)
	if len(req.Command) > 0 {
		cmd.Args[0] = req.Command[0]
	}
	captureLimit := effectiveOutputCaptureLimit("redacted", s.execPolicy.MaxOutputBytes)
	out, exitCode, err := runCommandWithBoundedOutput(cmd, captureLimit)
	if err != nil {
		writeMappedError(w, ErrInternal, err.Error())
		return
	}

	at, aid := actorFromRequest(r)
	_ = s.svc.Audit.Write(ports.AuditEvent{
		Event:     "host_docker_execute",
		Timestamp: s.now(),
		ActorType: at,
		ActorID:   aid,
		Metadata: map[string]string{
			"command":   strings.Join(req.Command, " "),
			"exit_code": itoa(uint64(exitCode)),
		},
	})
	outStr := redactOutput(out)
	if max := s.execPolicy.MaxOutputBytes; max > 0 && len(outStr) > max {
		outStr = outStr[:max]
	}
	writeJSON(w, map[string]any{"exit_code": exitCode, "stdout_stderr": outStr})
}

func (s *server) validateHostDockerCommand(cmd []string) error {
	return s.controlPolicy().ValidateHostDockerCommand(cmd)
}
