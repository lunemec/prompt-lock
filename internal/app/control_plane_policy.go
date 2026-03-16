package app

import (
	"fmt"
	"strings"

	"github.com/lunemec/promptlock/internal/config"
)

type ControlPlanePolicy interface {
	ValidateExecuteRequest(securityProfile string, req ExecuteRequest) error
	ValidateExecuteCommand(cmd []string) error
	ResolveExecuteCommand(cmd []string) (ResolvedCommand, error)
	ValidateNetworkEgress(cmd []string, intent string) error
	ValidateHostDockerCommand(cmd []string) error
	ResolveHostDockerCommand(cmd []string) (ResolvedCommand, error)
	ApplyOutputSecurity(in string) string
	ClampTimeout(requested int) int
}

type ExecuteRequest struct {
	Intent  string
	Command []string
}

type DefaultControlPlanePolicy struct {
	Exec       config.ExecutionPolicy
	HostOps    config.HostOpsPolicy
	Network    config.NetworkEgressPolicy
	OutputMode string
	DefaultTO  int
	MaxTO      int
}

func NewDefaultControlPlanePolicy(exec config.ExecutionPolicy, host config.HostOpsPolicy, netpol config.NetworkEgressPolicy) DefaultControlPlanePolicy {
	return DefaultControlPlanePolicy{
		Exec:       exec,
		HostOps:    host,
		Network:    netpol,
		OutputMode: exec.OutputSecurityMode,
		DefaultTO:  exec.DefaultTimeoutSec,
		MaxTO:      exec.MaxTimeoutSec,
	}
}

func (p DefaultControlPlanePolicy) ValidateExecuteRequest(securityProfile string, req ExecuteRequest) error {
	if securityProfile == "hardened" {
		if strings.TrimSpace(req.Intent) == "" {
			return fmt.Errorf("intent is required in hardened profile")
		}
		if len(req.Command) > 0 {
			cmd0 := strings.ToLower(strings.TrimSpace(req.Command[0]))
			if cmd0 == "bash" || cmd0 == "sh" || cmd0 == "zsh" {
				return fmt.Errorf("raw shell wrappers are not allowed in hardened profile")
			}
		}
	}
	return p.ValidateExecuteCommand(req.Command)
}

func (p DefaultControlPlanePolicy) ValidateExecuteCommand(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd0 := ExecutableIdentity(cmd[0])
	allowed := false
	for _, allowedExec := range p.Exec.ExactMatchExecutables {
		if cmd0 != "" && cmd0 == ExecutableIdentity(allowedExec) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("command %q not allowed by execution policy", cmd0)
	}
	joined := strings.ToLower(strings.Join(cmd, " "))
	for _, d := range p.Exec.DenylistSubstrings {
		if d != "" && strings.Contains(joined, strings.ToLower(d)) {
			return fmt.Errorf("command denied by policy substring %q", d)
		}
	}
	return nil
}

func (p DefaultControlPlanePolicy) ResolveExecuteCommand(cmd []string) (ResolvedCommand, error) {
	if err := p.ValidateExecuteCommand(cmd); err != nil {
		return ResolvedCommand{}, err
	}
	return ResolveExecutionCommand(cmd, p.Exec.CommandSearchPaths)
}

func (p DefaultControlPlanePolicy) ValidateNetworkEgress(cmd []string, intent string) error {
	if !p.Network.Enabled {
		return nil
	}
	joined := strings.ToLower(strings.Join(cmd, " "))
	for _, d := range p.Network.DenySubstrings {
		if d != "" && strings.Contains(joined, strings.ToLower(d)) {
			return fmt.Errorf("network egress denied by substring %q", d)
		}
	}
	domains, unsafeArg := extractDomains(cmd)
	if len(domains) == 0 {
		if requiresInspectableNetworkDestination(cmd) {
			return fmt.Errorf("network egress requires an inspectable destination in argv")
		}
		return nil
	}
	if unsafeArg != "" {
		return fmt.Errorf("network egress argv inspection cannot safely enforce opaque or destination-override argument %q", unsafeArg)
	}
	allow := p.Network.AllowDomains
	trimIntent := strings.TrimSpace(intent)
	if trimIntent != "" {
		if byIntent, ok := p.Network.IntentAllowDomains[trimIntent]; ok && len(byIntent) > 0 {
			allow = byIntent
		}
	} else if p.Network.RequireIntentMatch {
		return fmt.Errorf("network egress requires intent-specific policy")
	}
	if len(allow) == 0 {
		return fmt.Errorf("network egress disabled: no allowed domains for intent %q", trimIntent)
	}
	for _, dom := range domains {
		if !domainAllowed(dom, allow) {
			return fmt.Errorf("domain %q not allowed by network policy", dom)
		}
	}
	return nil
}

func (p DefaultControlPlanePolicy) ValidateHostDockerCommand(cmd []string) error {
	if len(cmd) < 2 {
		return fmt.Errorf("command must include docker and subcommand")
	}
	if cmd[0] != "docker" {
		return fmt.Errorf("only docker command is allowed")
	}
	sub := strings.TrimSpace(strings.ToLower(cmd[1]))
	if !containsCI(p.HostOps.DockerAllowSubcommands, sub) {
		return fmt.Errorf("docker subcommand %q not allowed", sub)
	}
	joined := strings.ToLower(strings.Join(cmd, " "))
	for _, d := range p.HostOps.DockerDenySubstrings {
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
		if err := validateFlags(cmd[2:], p.HostOps.DockerPSAllowedFlags); err != nil {
			return fmt.Errorf("docker ps: %w", err)
		}
	case "images":
		if err := validateFlags(cmd[2:], p.HostOps.DockerImagesAllowedFlags); err != nil {
			return fmt.Errorf("docker images: %w", err)
		}
	case "compose":
		if len(cmd) < 3 {
			return fmt.Errorf("docker compose requires verb")
		}
		verb := strings.ToLower(strings.TrimSpace(cmd[2]))
		if !containsCI(p.HostOps.DockerComposeAllowVerbs, verb) {
			return fmt.Errorf("docker compose verb %q not allowed", verb)
		}
		if err := validateFlags(cmd[3:], []string{"--project-name", "-p", "--file", "-f", "--profiles", "--profile", "--ansi", "--progress"}); err != nil {
			return fmt.Errorf("docker compose: %w", err)
		}
	}
	return nil
}

func (p DefaultControlPlanePolicy) ResolveHostDockerCommand(cmd []string) (ResolvedCommand, error) {
	if err := p.ValidateHostDockerCommand(cmd); err != nil {
		return ResolvedCommand{}, err
	}
	return ResolveExecutionCommand(cmd, p.Exec.CommandSearchPaths)
}

func (p DefaultControlPlanePolicy) ApplyOutputSecurity(in string) string {
	switch strings.ToLower(strings.TrimSpace(p.OutputMode)) {
	case "none":
		return ""
	case "raw":
		return in
	case "redacted", "":
		return redactOutput(in)
	default:
		return redactOutput(in)
	}
}

func (p DefaultControlPlanePolicy) ClampTimeout(requested int) int {
	timeout := requested
	if timeout <= 0 {
		timeout = p.DefaultTO
	}
	if timeout > p.MaxTO {
		timeout = p.MaxTO
	}
	return timeout
}

func requiresInspectableNetworkDestination(cmd []string) bool {
	if len(cmd) == 0 {
		return false
	}
	_, ok := directNetworkClients[ExecutableIdentity(cmd[0])]
	return ok
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
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		return fmt.Errorf("positional argument %q not allowed", a)
	}
	return nil
}
