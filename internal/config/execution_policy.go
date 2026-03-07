package config

type ExecutionPolicy struct {
	AllowlistPrefixes  []string `json:"allowlist_prefixes"`
	DenylistSubstrings []string `json:"denylist_substrings"`
	MaxOutputBytes     int      `json:"max_output_bytes"`
	DefaultTimeoutSec  int      `json:"default_timeout_sec"`
	MaxTimeoutSec      int      `json:"max_timeout_sec"`
}

func defaultExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{
		AllowlistPrefixes:  []string{"bash", "sh", "npm", "node", "go", "python", "pytest", "make", "git"},
		DenylistSubstrings: []string{"printenv", "/proc/", "environ", "aws_secret_access_key", "OPENAI_API_KEY"},
		MaxOutputBytes:     65536,
		DefaultTimeoutSec:  120,
		MaxTimeoutSec:      600,
	}
}
