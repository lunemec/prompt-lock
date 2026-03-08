package config

type NetworkEgressPolicy struct {
	Enabled            bool                `json:"enabled"`
	RequireIntentMatch bool                `json:"require_intent_match"`
	AllowDomains       []string            `json:"allow_domains"`
	IntentAllowDomains map[string][]string `json:"intent_allow_domains"`
	DenySubstrings     []string            `json:"deny_substrings"`
}

func defaultNetworkEgressPolicy() NetworkEgressPolicy {
	return NetworkEgressPolicy{
		Enabled:            false,
		RequireIntentMatch: false,
		AllowDomains:       []string{},
		IntentAllowDomains: map[string][]string{},
		DenySubstrings:     []string{"169.254.169.254", "metadata.google.internal", "localhost", "127.0.0.1"},
	}
}
