package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestValidateHostDockerCommand(t *testing.T) {
	s := &server{hostOpsPolicy: config.HostOpsPolicy{
		DockerAllowSubcommands:   []string{"version", "ps", "images", "compose"},
		DockerComposeAllowVerbs:  []string{"config", "ps", "ls", "images"},
		DockerPSAllowedFlags:     []string{"-a", "--all", "-q", "--quiet", "--format", "-f", "--filter", "--no-trunc", "-n", "--last"},
		DockerImagesAllowedFlags: []string{"-q", "--quiet", "--format", "--no-trunc", "--digests", "--filter"},
		DockerDenySubstrings:     []string{"--privileged", "--pid=host", "docker.sock", "--mount", ";"},
	}}
	if err := s.validateHostDockerCommand([]string{"docker", "ps", "-a"}); err != nil {
		t.Fatalf("expected allow: %v", err)
	}
	if err := s.validateHostDockerCommand([]string{"docker", "ps", "--danger"}); err == nil {
		t.Fatalf("expected flag rejection")
	}
	if err := s.validateHostDockerCommand([]string{"docker", "run", "alpine"}); err == nil {
		t.Fatalf("expected disallow for run")
	}
	if err := s.validateHostDockerCommand([]string{"docker", "compose", "up"}); err == nil {
		t.Fatalf("expected compose verb rejection")
	}
	if err := s.validateHostDockerCommand([]string{"docker", "compose", "ps", "--project-name", "x"}); err != nil {
		t.Fatalf("expected allowed compose command: %v", err)
	}
	if err := s.validateHostDockerCommand([]string{"docker", "compose", "ps", "--project-name", "x;rm"}); err == nil {
		t.Fatalf("expected smuggling/substring rejection")
	}
}
