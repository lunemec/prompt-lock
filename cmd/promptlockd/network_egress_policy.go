package main

import (
	"fmt"
	"net/url"
	"strings"
)

func (s *server) validateNetworkEgress(cmd []string, intent string) error {
	p := s.networkEgressPolicy
	if !p.Enabled {
		return nil
	}
	joined := strings.ToLower(strings.Join(cmd, " "))
	for _, d := range p.DenySubstrings {
		if d != "" && strings.Contains(joined, strings.ToLower(d)) {
			return fmt.Errorf("network egress denied by substring %q", d)
		}
	}

	domains := extractDomains(cmd)
	if len(domains) == 0 {
		return nil
	}

	allow := p.AllowDomains
	trimIntent := strings.TrimSpace(intent)
	if trimIntent != "" {
		if byIntent, ok := p.IntentAllowDomains[trimIntent]; ok && len(byIntent) > 0 {
			allow = byIntent
		}
	} else if p.RequireIntentMatch {
		return fmt.Errorf("network egress requires intent-specific policy")
	}

	if len(allow) == 0 {
		return fmt.Errorf("network egress disabled: no allowed domains for intent %q", trimIntent)
	}

	for _, dom := range domains {
		if !domainAllowed(dom, allow) {
			return fmt.Errorf("domain %q not allowed by network policy", dom)
		}
	}
	return nil
}

func extractDomains(cmd []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, part := range cmd {
		if strings.Contains(part, "://") {
			u, err := url.Parse(part)
			if err == nil && u.Hostname() != "" {
				h := strings.ToLower(u.Hostname())
				if !seen[h] {
					seen[h] = true
					out = append(out, h)
				}
			}
		}
	}
	return out
}

func domainAllowed(domain string, allow []string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	for _, a := range allow {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if d == a || strings.HasSuffix(d, "."+a) {
			return true
		}
	}
	return false
}
