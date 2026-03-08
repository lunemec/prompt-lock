package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestValidateHostDockerCommand(t *testing.T) {
	s := &server{hostOpsPolicy: config.HostOpsPolicy{
		DockerAllowSubcommands: []string{"version", "ps", "images", "compose"},
		DockerDenySubstrings:   []string{"--privileged", "--pid=host", "docker.sock"},
	}}
	if err := s.validateHostDockerCommand([]string{"docker", "ps"}); err != nil {
		t.Fatalf("expected allow: %v", err)
	}
	if err := s.validateHostDockerCommand([]string{"docker", "run", "alpine"}); err == nil {
		t.Fatalf("expected disallow for run")
	}
	if err := s.validateHostDockerCommand([]string{"docker", "compose", "up", "--privileged"}); err == nil {
		t.Fatalf("expected denylist block")
	}
}
