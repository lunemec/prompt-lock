package main

import (
	"fmt"
	"net"
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
	add := func(host string) {
		h := strings.ToLower(strings.TrimSpace(host))
		h = strings.Trim(h, "[]")
		if h == "" {
			return
		}
		if i := strings.Index(h, ":"); i > 0 {
			h = h[:i]
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}

	for i, part := range cmd {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if strings.Contains(p, "://") {
			u, err := url.Parse(p)
			if err == nil && u.Hostname() != "" {
				add(u.Hostname())
			}
			continue
		}
		lower := strings.ToLower(p)
		if lower == "--url" || lower == "-u" || lower == "--host" || lower == "-h" {
			if i+1 < len(cmd) {
				next := strings.TrimSpace(cmd[i+1])
				if strings.Contains(next, "://") {
					u, err := url.Parse(next)
					if err == nil && u.Hostname() != "" {
						add(u.Hostname())
					}
				} else if isDomainLike(next) {
					add(next)
				}
			}
			continue
		}
		if strings.HasPrefix(lower, "--host=") {
			add(strings.TrimPrefix(p, "--host="))
			continue
		}
		if isDomainLike(p) {
			add(p)
		}
	}
	return out
}

func domainAllowed(domain string, allow []string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	if isBlockedIPTarget(d) {
		return false
	}
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

func isDomainLike(s string) bool {
	s = strings.Trim(strings.ToLower(strings.TrimSpace(s)), "[]")
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") || strings.ContainsAny(s, " \\t\\n\\r") {
		return false
	}
	if ip := net.ParseIP(s); ip != nil {
		return true
	}
	return strings.Contains(s, ".")
}

func isBlockedIPTarget(host string) bool {
	h := strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return false
}
