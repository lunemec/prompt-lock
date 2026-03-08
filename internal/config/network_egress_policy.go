package config

type NetworkEgressPolicy struct {
	Enabled        bool     `json:"enabled"`
	AllowDomains   []string `json:"allow_domains"`
	DenySubstrings []string `json:"deny_substrings"`
}

func defaultNetworkEgressPolicy() NetworkEgressPolicy {
	return NetworkEgressPolicy{
		Enabled:        false,
		AllowDomains:   []string{},
		DenySubstrings: []string{"169.254.169.254", "metadata.google.internal", "localhost", "127.0.0.1"},
	}
}
