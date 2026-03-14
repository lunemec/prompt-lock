package main

import "strings"

func withPolicyHint(msg string) string {
	m := strings.ToLower(msg)
	hint := ""
	switch {
	case strings.Contains(m, "intent is required"):
		hint = "provide intent field matching configured intent_allow_domains"
	case strings.Contains(m, "raw shell wrappers"):
		hint = "use direct tool command (go/npm/python/git) instead of bash/sh wrapper"
	case strings.Contains(m, "not allowed by execution policy"):
		hint = "check execution_policy.exact_match_executables in config"
	case strings.Contains(m, "denied by policy substring"):
		hint = "remove denied token or adjust policy denylist consciously"
	case strings.Contains(m, "network egress") || strings.Contains(m, "domain"):
		hint = "add allowed destination under network_egress_policy.intent_allow_domains for this intent"
	case strings.Contains(m, "compose verb"):
		hint = "use allowed docker compose verbs from host_ops_policy.docker_compose_allow_verbs"
	case strings.Contains(m, "flag") && strings.Contains(m, "not allowed"):
		hint = "use only allowlisted flags from host_ops_policy"
	}
	if hint == "" {
		return msg
	}
	return msg + "; hint: " + hint
}
