package config

import "runtime"

type ExecutionPolicy struct {
	ExactMatchExecutables []string `json:"exact_match_executables"`
	CommandSearchPaths    []string `json:"command_search_paths"`
	DenylistSubstrings    []string `json:"denylist_substrings"`
	OutputSecurityMode    string   `json:"output_security_mode"`
	MaxOutputBytes        int      `json:"max_output_bytes"`
	DefaultTimeoutSec     int      `json:"default_timeout_sec"`
	MaxTimeoutSec         int      `json:"max_timeout_sec"`
}

func defaultExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{
		ExactMatchExecutables: []string{"bash", "sh", "npm", "node", "go", "python", "python3", "pytest", "make", "git"},
		CommandSearchPaths:    defaultCommandSearchPaths(),
		DenylistSubstrings:    []string{"printenv", "/proc/", "environ", "aws_secret_access_key", "OPENAI_API_KEY"},
		OutputSecurityMode:    "redacted",
		MaxOutputBytes:        65536,
		DefaultTimeoutSec:     120,
		MaxTimeoutSec:         600,
	}
}

func defaultCommandSearchPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{
			`C:\Windows\System32`,
			`C:\Windows`,
			`C:\Windows\System32\WindowsPowerShell\v1.0`,
		}
	}
	return []string{
		"/usr/local/bin",
		"/usr/local/sbin",
		"/usr/local/go/bin",
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/opt/local/bin",
		"/opt/local/sbin",
		"/usr/bin",
		"/usr/sbin",
		"/bin",
		"/sbin",
	}
}
