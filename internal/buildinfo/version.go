package buildinfo

import (
	"regexp"
	"runtime/debug"
	"strings"
)

var pseudoVersionPattern = regexp.MustCompile(`(?:-|\.)(\d{14})-[0-9a-fA-F]{12}(?:\+dirty)?$`)

func ResolveVersion(explicit string) string {
	return resolveVersion(explicit, debug.ReadBuildInfo)
}

func resolveVersion(explicit string, readInfo func() (*debug.BuildInfo, bool)) string {
	if v := normalizeVersion(explicit); v != "" {
		return v
	}
	if readInfo != nil {
		if info, ok := readInfo(); ok && info != nil {
			if v := normalizeVersion(info.Main.Version); v != "" {
				return v
			}
		}
	}
	return "dev"
}

func normalizeVersion(v string) string {
	trimmed := strings.TrimSpace(v)
	switch trimmed {
	case "", "dev", "(devel)":
		return ""
	default:
		if pseudoVersionPattern.MatchString(trimmed) {
			return ""
		}
		return strings.TrimPrefix(trimmed, "v")
	}
}
