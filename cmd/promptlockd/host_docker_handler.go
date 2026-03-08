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
	if !containsCI(s.hostOpsPolicy.DockerAllowSubcommands, sub) {
		return fmt.Errorf("docker subcommand %q not allowed", sub)
	}

	joined := strings.ToLower(strings.Join(cmd, " "))
	for _, d := range s.hostOpsPolicy.DockerDenySubstrings {
		if d != "" && strings.Contains(joined, strings.ToLower(d)) {
			return fmt.Errorf("docker command denied by policy substring %q", d)
		}
	}

	switch sub {
	case "version":
		if len(cmd) > 2 {
			return fmt.Errorf("docker version does not allow extra args in this policy")
		}
	case "ps":
		if err := validateFlags(cmd[2:], s.hostOpsPolicy.DockerPSAllowedFlags); err != nil {
			return fmt.Errorf("docker ps: %w", err)
		}
	case "images":
		if err := validateFlags(cmd[2:], s.hostOpsPolicy.DockerImagesAllowedFlags); err != nil {
			return fmt.Errorf("docker images: %w", err)
		}
	case "compose":
		if len(cmd) < 3 {
			return fmt.Errorf("docker compose requires verb")
		}
		verb := strings.ToLower(strings.TrimSpace(cmd[2]))
		if !containsCI(s.hostOpsPolicy.DockerComposeAllowVerbs, verb) {
			return fmt.Errorf("docker compose verb %q not allowed", verb)
		}
		if err := validateFlags(cmd[3:], []string{"--project-name", "-p", "--file", "-f", "--profiles", "--profile", "--ansi", "--progress"}); err != nil {
			return fmt.Errorf("docker compose: %w", err)
		}
	}

	return nil
}

func containsCI(items []string, needle string) bool {
	n := strings.ToLower(strings.TrimSpace(needle))
	for _, it := range items {
		if strings.ToLower(strings.TrimSpace(it)) == n {
			return true
		}
	}
	return false
}

func validateFlags(args []string, allow []string) error {
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "-") {
			if !containsCI(allow, a) {
				return fmt.Errorf("flag %q not allowed", a)
			}
			// skip flag value for known value-taking flags
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		return fmt.Errorf("positional argument %q not allowed", a)
	}
	return nil
}
