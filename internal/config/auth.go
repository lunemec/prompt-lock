package config

type AuthConfig struct {
	EnableAuth               bool `json:"enable_auth"`
	SessionTTLMinutes        int  `json:"session_ttl_minutes"`
	GrantIdleTimeoutMinutes  int  `json:"grant_idle_timeout_minutes"`
	GrantAbsoluteMaxMinutes  int  `json:"grant_absolute_max_minutes"`
	BootstrapTokenTTLSeconds int  `json:"bootstrap_token_ttl_seconds"`
}

func defaultAuthConfig() AuthConfig {
	return AuthConfig{
		EnableAuth:               false,
		SessionTTLMinutes:        10,
		GrantIdleTimeoutMinutes:  480,
		GrantAbsoluteMaxMinutes:  10080,
		BootstrapTokenTTLSeconds: 60,
	}
}
