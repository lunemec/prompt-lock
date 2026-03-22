package mcplaunchenv

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

const DefaultFilePath = "/run/promptlock/promptlock-mcp.env"

func FilePathFromEnv(getenv func(string) string) string {
	if getenv != nil {
		if override := strings.TrimSpace(getenv("PROMPTLOCK_MCP_ENV_FILE")); override != "" {
			return override
		}
	}
	return DefaultFilePath
}

func Parse(data []byte) (map[string]string, error) {
	values := map[string]string{}
	for i, rawLine := range bytes.Split(data, []byte("\n")) {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("line %d is not KEY=VALUE", i+1)
		}
		values[strings.TrimSpace(name)] = value
	}
	return values, nil
}

func Format(values map[string]string, order []string) ([]byte, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("wrapper MCP env values are required")
	}
	seen := map[string]struct{}{}
	lines := make([]string, 0, len(values))
	for _, key := range order {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value, ok := values[key]
		if !ok {
			continue
		}
		if strings.Contains(value, "\n") {
			return nil, fmt.Errorf("%s contains newline", key)
		}
		lines = append(lines, key+"="+value)
		seen[key] = struct{}{}
	}
	extra := make([]string, 0, len(values))
	for key := range values {
		if _, ok := seen[key]; ok {
			continue
		}
		extra = append(extra, key)
	}
	sort.Strings(extra)
	for _, key := range extra {
		value := values[key]
		if strings.Contains(value, "\n") {
			return nil, fmt.Errorf("%s contains newline", key)
		}
		lines = append(lines, key+"="+value)
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}
