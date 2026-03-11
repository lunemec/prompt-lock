package config

type RequestPolicyConfig struct {
	IdenticalRequestCooldownSeconds int  `json:"identical_request_cooldown_seconds"`
	MaxPendingPerAgent              int  `json:"max_pending_per_agent"`
	EnableActiveLeaseReuse          bool `json:"enable_active_lease_reuse"`
}

func defaultRequestPolicyConfig() RequestPolicyConfig {
	return RequestPolicyConfig{
		IdenticalRequestCooldownSeconds: 60,
		MaxPendingPerAgent:              2,
		EnableActiveLeaseReuse:          true,
	}
}
