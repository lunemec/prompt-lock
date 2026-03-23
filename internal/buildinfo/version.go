package buildinfo

import (
	"runtime/debug"
	"strings"
)

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
			if rev := buildSetting(info, "vcs.revision"); rev != "" {
				short := rev
				if len(short) > 12 {
					short = short[:12]
				}
				if buildSetting(info, "vcs.modified") == "true" {
					return "dev+" + short + "-dirty"
				}
				return "dev+" + short
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
		return strings.TrimPrefix(trimmed, "v")
	}
}

func buildSetting(info *debug.BuildInfo, key string) string {
	if info == nil {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return strings.TrimSpace(setting.Value)
		}
	}
	return ""
}
