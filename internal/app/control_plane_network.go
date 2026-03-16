package app

import (
	"net"
	"net/url"
	"strings"
)

func extractDomains(cmd []string) ([]string, string) {
	if len(cmd) == 0 {
		return nil, ""
	}
	if spec, ok := directClientArgSpecs[ExecutableIdentity(cmd[0])]; ok {
		return extractDirectClientDomains(cmd[1:], spec)
	}
	return extractGenericDomains(cmd), ""
}

func extractGenericDomains(cmd []string) []string {
	seen := map[string]bool{}
	out := []string{}
	addCandidate := func(candidate string) {
		host := extractDomainHost(candidate)
		if host == "" {
			return
		}
		if !seen[host] {
			seen[host] = true
			out = append(out, host)
		}
	}
	for i := 0; i < len(cmd); i++ {
		part := cmd[i]
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		lower := strings.ToLower(p)
		if lower == "--url" {
			if i+1 < len(cmd) {
				i++
				addCandidate(cmd[i])
			}
			continue
		}
		if strings.HasPrefix(lower, "--url=") {
			if _, value, ok := strings.Cut(p, "="); ok {
				addCandidate(value)
			}
			continue
		}
		addCandidate(p)
	}
	return out
}

func extractDirectClientDomains(args []string, spec directClientArgSpec) ([]string, string) {
	seen := map[string]bool{}
	out := []string{}
	unsafeArg := ""
	addCandidate := func(candidate string) {
		host := extractDomainHost(candidate)
		if host == "" {
			return
		}
		if !seen[host] {
			seen[host] = true
			out = append(out, host)
		}
	}
	recordUnsafeArg := func(flag string) {
		if unsafeArg == "" {
			unsafeArg = flag
		}
	}

	for i := 0; i < len(args); i++ {
		part := strings.TrimSpace(args[i])
		if part == "" {
			continue
		}
		if part == "--" {
			for _, remaining := range args[i+1:] {
				addCandidate(remaining)
			}
			break
		}
		if !strings.HasPrefix(part, "-") {
			addCandidate(part)
			continue
		}
		if strings.HasPrefix(part, "--") {
			name, inlineValue, hasInlineValue := splitLongFlagToken(part)
			mode, classified := spec.longFlags[strings.ToLower(name)]
			if !classified {
				recordUnsafeArg(name)
				if !hasInlineValue && i+1 < len(args) && !looksLikeFlag(args[i+1]) {
					i++
				}
				continue
			}
			switch mode {
			case directClientFlagDestination:
				switch {
				case hasInlineValue:
					addCandidate(inlineValue)
				case i+1 < len(args):
					i++
					addCandidate(args[i])
				}
			case directClientFlagOpaqueOrOverrideValue:
				recordUnsafeArg(name)
				if !hasInlineValue && i+1 < len(args) {
					i++
				}
			case directClientFlagTakesValue:
				if !hasInlineValue && i+1 < len(args) {
					i++
				}
			case directClientFlagNoValue:
			default:
				if !hasInlineValue && i+1 < len(args) && !looksLikeFlag(args[i+1]) {
					i++
				}
			}
			continue
		}
		destination, consumedNext, shortUnsafeArg := consumeShortFlagToken(part, args, i, spec)
		if shortUnsafeArg != "" {
			recordUnsafeArg(shortUnsafeArg)
		}
		if consumedNext {
			i++
			if destination != "" {
				addCandidate(destination)
			}
			continue
		}
		if destination != "" {
			addCandidate(destination)
		}
	}
	return out, unsafeArg
}

func splitLongFlagToken(part string) (name, inlineValue string, hasInlineValue bool) {
	if before, after, ok := strings.Cut(part, "="); ok {
		return before, after, true
	}
	return part, "", false
}

func consumeShortFlagToken(part string, args []string, index int, spec directClientArgSpec) (destination string, consumedNext bool, unsafeFlag string) {
	if part == "-" {
		return "", false, ""
	}
	cluster := part[1:]
	for i := 0; i < len(cluster); i++ {
		flag := cluster[i]
		mode, classified := spec.shortFlags[flag]
		if !classified {
			flagName := "-" + string(flag)
			if i+1 < len(cluster) {
				return "", false, flagName
			}
			if index+1 < len(args) && !looksLikeFlag(args[index+1]) {
				return "", true, flagName
			}
			return "", false, flagName
		}
		switch mode {
		case directClientFlagDestination:
			if i+1 < len(cluster) {
				return cluster[i+1:], false, ""
			}
			if index+1 < len(args) {
				return args[index+1], true, ""
			}
			return "", false, ""
		case directClientFlagOpaqueOrOverrideValue:
			flagName := "-" + string(flag)
			if i+1 < len(cluster) {
				return "", false, flagName
			}
			if index+1 < len(args) {
				return "", true, flagName
			}
			return "", false, flagName
		case directClientFlagTakesValue:
			if i+1 < len(cluster) {
				return "", false, ""
			}
			if index+1 < len(args) {
				return "", true, ""
			}
			return "", false, ""
		case directClientFlagNoValue:
			continue
		}
	}
	return "", false, ""
}

func looksLikeFlag(candidate string) bool {
	return strings.HasPrefix(strings.TrimSpace(candidate), "-")
}

func extractDomainHost(candidate string) string {
	p := strings.TrimSpace(candidate)
	if p == "" {
		return ""
	}
	if strings.Contains(p, "://") {
		u, err := url.Parse(p)
		if err == nil && u.Hostname() != "" {
			return normalizeDomainHost(u.Hostname())
		}
		return ""
	}
	if host, ok := splitHostPathCandidate(p); ok {
		return normalizeDomainHost(host)
	}
	if isDomainLike(p) {
		return normalizeDomainHost(p)
	}
	return ""
}

func normalizeDomainHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return ""
	}
	if strings.HasPrefix(h, "[") {
		end := strings.Index(h, "]")
		if end <= 1 {
			return ""
		}
		literal := h[1:end]
		if ip := net.ParseIP(literal); ip == nil {
			return ""
		}
		rest := h[end+1:]
		if rest == "" {
			return literal
		}
		if strings.HasPrefix(rest, ":") && isDecimalString(rest[1:]) {
			return literal
		}
		return ""
	}
	h = strings.Trim(h, "[]")
	if h == "" || strings.ContainsAny(h, "/ \t\n\r") {
		return ""
	}
	if ip := net.ParseIP(h); ip != nil {
		return h
	}
	if strings.Count(h, ":") == 1 {
		hostOnly, port, ok := strings.Cut(h, ":")
		if !ok || hostOnly == "" || !isDecimalString(port) {
			return ""
		}
		h = hostOnly
	}
	if strings.Contains(h, ":") || !strings.Contains(h, ".") {
		return ""
	}
	return h
}

func isDecimalString(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
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
	if strings.Contains(s, "/") || strings.ContainsAny(s, " \t\n\r") {
		return false
	}
	if ip := net.ParseIP(s); ip != nil {
		return true
	}
	return strings.Contains(s, ".")
}

func splitHostPathCandidate(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "-") {
		return "", false
	}
	slash := strings.Index(trimmed, "/")
	if slash <= 0 {
		return "", false
	}
	host := normalizeDomainHost(trimmed[:slash])
	if host != "" {
		return host, true
	}
	return "", false
}

func isBlockedIPTarget(host string) bool {
	h := strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
