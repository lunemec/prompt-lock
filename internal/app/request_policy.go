package app

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

const (
	defaultIdenticalRequestCooldown = 60 * time.Second
	defaultMaxPendingPerAgent       = 2
)

type RequestPolicy struct {
	IdenticalRequestCooldown time.Duration
	MaxPendingPerAgent       int
	EnableActiveLeaseReuse   bool
}

type RequestPolicyInput struct {
	AgentID            string
	Intent             string
	Secrets            []string
	CommandFingerprint string
	WorkdirFingerprint string
}

func DefaultRequestPolicy() RequestPolicy {
	return RequestPolicy{
		IdenticalRequestCooldown: defaultIdenticalRequestCooldown,
		MaxPendingPerAgent:       defaultMaxPendingPerAgent,
		EnableActiveLeaseReuse:   true,
	}
}

func (p RequestPolicy) Normalize() RequestPolicy {
	out := p
	if out.IdenticalRequestCooldown <= 0 {
		out.IdenticalRequestCooldown = defaultIdenticalRequestCooldown
	}
	if out.MaxPendingPerAgent <= 0 {
		out.MaxPendingPerAgent = defaultMaxPendingPerAgent
	}
	return out
}

func (in RequestPolicyInput) EquivalenceKey() string {
	material := strings.Join([]string{
		strings.TrimSpace(in.AgentID),
		strings.TrimSpace(in.Intent),
		strings.Join(normalizeSecrets(in.Secrets), ","),
		strings.TrimSpace(in.CommandFingerprint),
		strings.TrimSpace(in.WorkdirFingerprint),
	}, "\x1f")
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

func normalizeSecrets(secrets []string) []string {
	if len(secrets) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(secrets))
	out := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		name := strings.TrimSpace(secret)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
