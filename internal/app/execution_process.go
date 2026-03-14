package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/lunemec/promptlock/internal/config"
)

var executionBaselineEnvAllowlist = []string{
	"PATH",
	"HOME",
	"TMPDIR",
	"TMP",
	"TEMP",
	"SYSTEMROOT",
	"COMSPEC",
	"PATHEXT",
	"USERPROFILE",
}

type ResolvedCommand struct {
	Path       string
	Args       []string
	SearchPath string
}

// BuildExecutionEnvironment deliberately carries only the minimum baseline
// runtime variables needed for common toolchains plus the explicitly leased
// secrets for the command.
func BuildExecutionEnvironment(ambient []string, leased map[string]string) []string {
	return BuildExecutionEnvironmentWithPathOverride(ambient, leased, "")
}

// BuildExecutionEnvironmentWithPathOverride keeps the same minimal baseline as
// BuildExecutionEnvironment while replacing PATH with a broker-managed value
// when one is provided.
func BuildExecutionEnvironmentWithPathOverride(ambient []string, leased map[string]string, pathOverride string) []string {
	selected := map[string]string{}
	for _, entry := range ambient {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		canonical := canonicalExecutionBaselineEnvKey(key)
		if canonical == "" {
			continue
		}
		selected[canonical] = value
	}
	if strings.TrimSpace(pathOverride) != "" {
		selected["PATH"] = pathOverride
	}

	out := make([]string, 0, len(executionBaselineEnvAllowlist)+len(leased))
	for _, key := range executionBaselineEnvAllowlist {
		value, ok := selected[key]
		if !ok {
			continue
		}
		out = append(out, key+"="+value)
	}

	secretKeys := make([]string, 0, len(leased))
	for name, value := range leased {
		envName := SecretEnvName(name)
		if envName == "" {
			continue
		}
		selected[envName] = value
		secretKeys = append(secretKeys, envName)
	}
	sort.Strings(secretKeys)
	for _, key := range secretKeys {
		out = append(out, key+"="+selected[key])
	}
	return out
}

func ResolveExecutionCommand(command []string, searchDirs []string) (ResolvedCommand, error) {
	if len(command) == 0 {
		return ResolvedCommand{}, fmt.Errorf("empty command")
	}
	normalizedRoots := normalizedSearchRoots(searchDirs)
	if len(normalizedRoots) == 0 {
		normalizedRoots = normalizedSearchRoots(config.Default().ExecutionPolicy.CommandSearchPaths)
	}
	if len(normalizedRoots) == 0 {
		return ResolvedCommand{}, fmt.Errorf("execution_policy.command_search_paths is empty")
	}
	resolvedPath, err := resolveExecutablePath(command[0], normalizedRoots)
	if err != nil {
		return ResolvedCommand{}, err
	}
	return ResolvedCommand{
		Path:       resolvedPath,
		Args:       append([]string{}, command[1:]...),
		SearchPath: strings.Join(normalizedRoots, string(os.PathListSeparator)),
	}, nil
}

func canonicalExecutionBaselineEnvKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	for _, allowed := range executionBaselineEnvAllowlist {
		if strings.EqualFold(trimmed, allowed) {
			return allowed
		}
	}
	return ""
}

func SecretEnvName(secretName string) string {
	trimmed := strings.TrimSpace(secretName)
	if trimmed == "" {
		return ""
	}
	return strings.ToUpper(trimmed)
}

func ExecutableIdentity(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ""
	}
	identity := strings.ToLower(filepath.Base(trimmed))
	if runtime.GOOS == "windows" {
		switch ext := strings.ToLower(filepath.Ext(identity)); ext {
		case ".exe", ".cmd", ".bat", ".com":
			identity = strings.TrimSuffix(identity, ext)
		}
	}
	return identity
}

func resolveExecutablePath(command string, allowedRoots []string) (string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", fmt.Errorf("empty command")
	}
	if containsPathSeparator(trimmed) || filepath.IsAbs(trimmed) {
		return resolveTrustedExecutablePath(trimmed, allowedRoots)
	}
	for _, root := range allowedRoots {
		for _, candidate := range executableCandidates(root, trimmed) {
			if !pathExists(candidate) {
				continue
			}
			return resolveTrustedExecutablePath(candidate, allowedRoots)
		}
	}
	return "", fmt.Errorf("command %q not found in execution_policy.command_search_paths", trimmed)
}

func resolveTrustedExecutablePath(candidate string, allowedRoots []string) (string, error) {
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve command path %q: %w", candidate, err)
	}
	absCandidate = filepath.Clean(absCandidate)
	if !pathWithinAnyRoot(absCandidate, allowedRoots) {
		return "", fmt.Errorf("command path %q is outside execution_policy.command_search_paths", absCandidate)
	}
	resolvedPath, err := filepath.EvalSymlinks(absCandidate)
	if err != nil {
		return "", fmt.Errorf("resolve command path %q: %w", candidate, err)
	}
	resolvedPath = filepath.Clean(resolvedPath)
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat command path %q: %w", resolvedPath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("command path %q is a directory", resolvedPath)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("command path %q is not executable", resolvedPath)
	}
	return resolvedPath, nil
}

func normalizedSearchRoots(searchDirs []string) []string {
	out := make([]string, 0, len(searchDirs))
	seen := map[string]struct{}{}
	for _, dir := range searchDirs {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			continue
		}
		absDir, err := filepath.Abs(trimmed)
		if err != nil {
			continue
		}
		resolvedDir, err := filepath.EvalSymlinks(absDir)
		if err != nil {
			resolvedDir = absDir
		}
		cleaned := filepath.Clean(resolvedDir)
		key := strings.ToLower(cleaned)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, cleaned)
		seen[key] = struct{}{}
	}
	return out
}

func pathWithinAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func pathWithinRoot(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func executableCandidates(root string, command string) []string {
	base := filepath.Join(root, command)
	if runtime.GOOS != "windows" {
		return []string{base}
	}
	if filepath.Ext(command) != "" {
		return []string{base}
	}
	exts := executableExtensions()
	out := make([]string, 0, len(exts)+1)
	out = append(out, base)
	for _, ext := range exts {
		out = append(out, base+ext)
	}
	return out
}

func executableExtensions() []string {
	raw := os.Getenv("PATHEXT")
	if strings.TrimSpace(raw) == "" {
		raw = ".EXE;.CMD;.BAT;.COM"
	}
	parts := strings.Split(raw, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, ".") {
			trimmed = "." + trimmed
		}
		out = append(out, strings.ToLower(trimmed))
	}
	return out
}

func containsPathSeparator(path string) bool {
	return strings.Contains(path, "/") || strings.Contains(path, "\\")
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
