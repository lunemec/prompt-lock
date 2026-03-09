package config

type AuthConfig struct {
	EnableAuth                 bool   `json:"enable_auth"`
	OperatorToken              string `json:"operator_token"`
	AllowPlaintextSecretReturn bool   `json:"allow_plaintext_secret_return"`
	SessionTTLMinutes          int    `json:"session_ttl_minutes"`
	GrantIdleTimeoutMinutes    int    `json:"grant_idle_timeout_minutes"`
	GrantAbsoluteMaxMinutes    int    `json:"grant_absolute_max_minutes"`
	BootstrapTokenTTLSeconds   int    `json:"bootstrap_token_ttl_seconds"`
	CleanupIntervalSeconds     int    `json:"cleanup_interval_seconds"`
	RateLimitWindowSeconds     int    `json:"rate_limit_window_seconds"`
	RateLimitMaxAttempts       int    `json:"rate_limit_max_attempts"`
	StoreFile                  string `json:"store_file"`
	StoreEncryptionKeyEnv      string `json:"store_encryption_key_env"`
}

func defaultAuthConfig() AuthConfig {
	return AuthConfig{
		EnableAuth:                 false,
		OperatorToken:              "",
		AllowPlaintextSecretReturn: true,
		SessionTTLMinutes:          10,
		GrantIdleTimeoutMinutes:    480,
		GrantAbsoluteMaxMinutes:    10080,
		BootstrapTokenTTLSeconds:   60,
		CleanupIntervalSeconds:     60,
		RateLimitWindowSeconds:     60,
		RateLimitMaxAttempts:       20,
		StoreFile:                  "",
		StoreEncryptionKeyEnv:      "PROMPTLOCK_AUTH_STORE_KEY",
	}
}
