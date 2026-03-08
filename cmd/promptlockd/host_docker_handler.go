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
		writeMappedError(w, ErrForbidden, err.Error())
		return
	}
	timeout := s.hostOpsPolicy.DockerTimeoutSec
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, req.Command[0], req.Command[1:]...)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			writeMappedError(w, ErrInternal, err.Error())
			return
		}
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
	writeJSON(w, map[string]any{"exit_code": exitCode, "stdout_stderr": redactOutput(string(out))})
}

func (s *server) validateHostDockerCommand(cmd []string) error {
	return s.controlPolicy().ValidateHostDockerCommand(cmd)
}
