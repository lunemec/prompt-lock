package domain

import "fmt"

type Policy struct {
	DefaultTTLMinutes int
	MinTTLMinutes     int
	MaxTTLMinutes     int
	MaxSecretsPerReq  int
}

func DefaultPolicy() Policy {
	return Policy{
		DefaultTTLMinutes: 5,
		MinTTLMinutes:     1,
		MaxTTLMinutes:     60,
		MaxSecretsPerReq:  5,
	}
}

func (p Policy) ValidateRequest(ttl int, secrets []string) error {
	if ttl < p.MinTTLMinutes || ttl > p.MaxTTLMinutes {
		return fmt.Errorf("ttl_minutes must be in range [%d, %d]", p.MinTTLMinutes, p.MaxTTLMinutes)
	}
	if len(secrets) == 0 {
		return fmt.Errorf("at least one secret required")
	}
	if len(secrets) > p.MaxSecretsPerReq {
		return fmt.Errorf("too many secrets requested (max %d)", p.MaxSecretsPerReq)
	}
	return nil
}
