package config

type SecretSourceConfig struct {
	Type             string `json:"type"`
	EnvPrefix        string `json:"env_prefix"`
	FilePath         string `json:"file_path"`
	InMemoryHardened string `json:"in_memory_hardened"` // warn|fail
}

func defaultSecretSourceConfig() SecretSourceConfig {
	return SecretSourceConfig{Type: "in_memory", EnvPrefix: "PROMPTLOCK_SECRET_", InMemoryHardened: "warn"}
}
