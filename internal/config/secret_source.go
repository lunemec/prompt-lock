package config

type SecretSourceConfig struct {
	Type                 string `json:"type"`
	EnvPrefix            string `json:"env_prefix"`
	FilePath             string `json:"file_path"`
	ExternalURL          string `json:"external_url"`
	ExternalAuthTokenEnv string `json:"external_auth_token_env"`
	ExternalTimeoutSec   int    `json:"external_timeout_sec"`
	InMemoryHardened     string `json:"in_memory_hardened"` // warn|fail
}

func defaultSecretSourceConfig() SecretSourceConfig {
	return SecretSourceConfig{
		Type:                 "in_memory",
		EnvPrefix:            "PROMPTLOCK_SECRET_",
		ExternalAuthTokenEnv: "PROMPTLOCK_EXTERNAL_SECRET_TOKEN",
		ExternalTimeoutSec:   10,
		InMemoryHardened:     "warn",
	}
}
