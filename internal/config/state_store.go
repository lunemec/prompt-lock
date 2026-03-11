package config

type StateStoreConfig struct {
	Type                 string `json:"type"`                    // file|external
	ExternalURL          string `json:"external_url"`            // used when type=external
	ExternalAuthTokenEnv string `json:"external_auth_token_env"` // bearer token env var
	ExternalTimeoutSec   int    `json:"external_timeout_sec"`    // timeout for external backend requests
}

func defaultStateStoreConfig() StateStoreConfig {
	return StateStoreConfig{
		Type:                 "file",
		ExternalAuthTokenEnv: "PROMPTLOCK_EXTERNAL_STATE_TOKEN",
		ExternalTimeoutSec:   10,
	}
}
