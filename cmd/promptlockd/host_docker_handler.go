package main

import (
	"context"
	"encoding/json"
	"fmt"
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
		http.Error(w, "method not allowed", 405)
		return
	}
	var req hostDockerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.validateHostDockerCommand(req.Command); err != nil {
		http.Error(w, err.Error(), 403)
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
			http.Error(w, err.Error(), 500)
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
	if len(cmd) < 2 {
		return fmt.Errorf("command must include docker and subcommand")
	}
	if cmd[0] != "docker" {
		return fmt.Errorf("only docker command is allowed")
	}
	sub := strings.TrimSpace(strings.ToLower(cmd[1]))
	allowed := false
	for _, a := range s.hostOpsPolicy.DockerAllowSubcommands {
		if sub == strings.ToLower(strings.TrimSpace(a)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("docker subcommand %q not allowed", sub)
	}
	joined := strings.ToLower(strings.Join(cmd, " "))
	for _, d := range s.hostOpsPolicy.DockerDenySubstrings {
		if d != "" && strings.Contains(joined, strings.ToLower(d)) {
			return fmt.Errorf("docker command denied by policy substring %q", d)
		}
	}
	return nil
}
