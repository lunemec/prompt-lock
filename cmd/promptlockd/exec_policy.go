package main

import (
	"fmt"
	"strings"
)

func (s *server) validateExecuteRequest(req executeReq) error {
	if s.securityProfile == "hardened" {
		if strings.TrimSpace(req.Intent) == "" {
			return fmt.Errorf("intent is required in hardened profile")
		}
		if len(req.Command) > 0 {
			cmd0 := strings.ToLower(strings.TrimSpace(req.Command[0]))
			if cmd0 == "bash" || cmd0 == "sh" || cmd0 == "zsh" {
				return fmt.Errorf("raw shell wrappers are not allowed in hardened profile")
			}
		}
	}
	return s.validateExecuteCommand(req.Command)
}

func (s *server) validateExecuteCommand(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd0 := strings.ToLower(strings.TrimSpace(cmd[0]))
	allowed := false
	for _, p := range s.execPolicy.AllowlistPrefixes {
		if strings.HasPrefix(cmd0, strings.ToLower(strings.TrimSpace(p))) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("command %q not allowed by execution policy", cmd0)
	}
	joined := strings.ToLower(strings.Join(cmd, " "))
	for _, d := range s.execPolicy.DenylistSubstrings {
		if d == "" {
			continue
		}
		if strings.Contains(joined, strings.ToLower(d)) {
			return fmt.Errorf("command denied by policy substring %q", d)
		}
	}
	return nil
}

func applyOutputSecurity(mode, in string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "none":
		return ""
	case "raw":
		return in
	case "redacted", "":
		return redactOutput(in)
	default:
		return redactOutput(in)
	}
}

func redactOutput(in string) string {
	s := in
	replacements := []string{
		"sk-", "[REDACTED_KEY_]",
		"api_key", "[REDACTED_API_KEY]",
		"secret", "[REDACTED_SECRET]",
	}
	for i := 0; i < len(replacements); i += 2 {
		s = strings.ReplaceAll(s, replacements[i], replacements[i+1])
	}
	return s
}
