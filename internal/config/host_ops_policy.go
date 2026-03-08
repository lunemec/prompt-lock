package config

type HostOpsPolicy struct {
	DockerAllowSubcommands []string `json:"docker_allow_subcommands"`
	DockerDenySubstrings   []string `json:"docker_deny_substrings"`
	DockerTimeoutSec       int      `json:"docker_timeout_sec"`
}

func defaultHostOpsPolicy() HostOpsPolicy {
	return HostOpsPolicy{
		DockerAllowSubcommands: []string{"version", "ps", "images", "compose"},
		DockerDenySubstrings:   []string{"--privileged", "--pid=host", "-v /:/", "docker.sock", "--network host"},
		DockerTimeoutSec:       30,
	}
}
