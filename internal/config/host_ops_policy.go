package config

type HostOpsPolicy struct {
	DockerAllowSubcommands   []string `json:"docker_allow_subcommands"`
	DockerComposeAllowVerbs  []string `json:"docker_compose_allow_verbs"`
	DockerPSAllowedFlags     []string `json:"docker_ps_allowed_flags"`
	DockerImagesAllowedFlags []string `json:"docker_images_allowed_flags"`
	DockerDenySubstrings     []string `json:"docker_deny_substrings"`
	DockerTimeoutSec         int      `json:"docker_timeout_sec"`
}

func defaultHostOpsPolicy() HostOpsPolicy {
	return HostOpsPolicy{
		DockerAllowSubcommands:   []string{"version", "ps", "images", "compose"},
		DockerComposeAllowVerbs:  []string{"config", "ps", "ls", "images"},
		DockerPSAllowedFlags:     []string{"-a", "--all", "-q", "--quiet", "--format", "-f", "--filter", "--no-trunc", "-n", "--last"},
		DockerImagesAllowedFlags: []string{"-q", "--quiet", "--format", "--no-trunc", "--digests", "--filter"},
		DockerDenySubstrings:     []string{"--privileged", "--pid=host", "-v /:/", "docker.sock", "--network host", "--mount", "-v "},
		DockerTimeoutSec:         30,
	}
}
