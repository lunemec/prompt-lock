package envpath

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store resolves request-scoped secrets from a .env file path constrained to a
// configured root directory.
type Store struct {
	root string
}

func New(root string) (*Store, error) {
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		return nil, fmt.Errorf("env-path store requires non-empty root")
	}
	canonicalRoot, err := canonicalPath(trimmedRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve env-path root: %w", err)
	}
	info, err := os.Stat(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("stat env-path root %q: %w", canonicalRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("env-path root %q is not a directory", canonicalRoot)
	}
	return &Store{root: canonicalRoot}, nil
}

func (s *Store) Resolve(envPath string, requestedKeys []string) (map[string]string, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("env-path store is nil")
	}
	keys, err := normalizeRequestedKeys(requestedKeys)
	if err != nil {
		return nil, "", err
	}
	canonicalTarget, err := s.resolveTargetPath(envPath)
	if err != nil {
		return nil, "", err
	}
	fh, err := os.Open(canonicalTarget)
	if err != nil {
		return nil, "", fmt.Errorf("open env file %q: %w", canonicalTarget, err)
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("stat env file %q: %w", canonicalTarget, err)
	}
	if info.IsDir() {
		return nil, "", fmt.Errorf("env path %q is a directory", canonicalTarget)
	}
	parsed, err := parseDotEnv(fh)
	if err != nil {
		return nil, "", fmt.Errorf("parse env file %q: %w", canonicalTarget, err)
	}
	resolved := map[string]string{}
	missing := []string{}
	for _, key := range keys {
		if value, ok := lookupSecretValue(parsed, key); ok {
			resolved[key] = value
			continue
		}
		missing = append(missing, key)
	}
	if len(missing) > 0 {
		return nil, "", fmt.Errorf("requested keys missing from env file: %s", strings.Join(missing, ", "))
	}
	return resolved, canonicalTarget, nil
}

func (s *Store) resolveTargetPath(envPath string) (string, error) {
	trimmedPath := strings.TrimSpace(envPath)
	if trimmedPath == "" {
		return "", fmt.Errorf("env path is required")
	}
	candidatePath := trimmedPath
	if !filepath.IsAbs(candidatePath) {
		candidatePath = filepath.Join(s.root, candidatePath)
	}
	canonicalTarget, err := canonicalPath(candidatePath)
	if err != nil {
		return "", fmt.Errorf("resolve env path %q: %w", trimmedPath, err)
	}
	if !withinRoot(s.root, canonicalTarget) {
		return "", fmt.Errorf("env path %q resolves outside allowed root", trimmedPath)
	}
	return canonicalTarget, nil
}

func canonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolvedPath), nil
}

func withinRoot(root, target string) bool {
	if filepath.Clean(root) == filepath.Clean(target) {
		return true
	}
	relPath, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	relPath = filepath.Clean(relPath)
	if relPath == "." {
		return true
	}
	if relPath == ".." {
		return false
	}
	return !strings.HasPrefix(relPath, ".."+string(os.PathSeparator))
}

func normalizeRequestedKeys(requestedKeys []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(requestedKeys))
	for _, key := range requestedKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("requested keys are required")
	}
	return out, nil
}

func lookupSecretValue(parsed map[string]string, requestedKey string) (string, bool) {
	if value, ok := parsed[requestedKey]; ok {
		return value, true
	}
	upper := strings.ToUpper(requestedKey)
	if value, ok := parsed[upper]; ok {
		return value, true
	}
	lower := strings.ToLower(requestedKey)
	if value, ok := parsed[lower]; ok {
		return value, true
	}
	return "", false
}

func parseDotEnv(file *os.File) (map[string]string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("seek env file: %w", err)
	}
	scanner := bufio.NewScanner(file)
	values := map[string]string{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		keyPart, valuePart, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("dotenv line %d missing '='", lineNo)
		}
		key := strings.TrimSpace(keyPart)
		if key == "" {
			return nil, fmt.Errorf("dotenv line %d has empty key", lineNo)
		}
		value := strings.TrimSpace(valuePart)
		if len(value) >= 2 {
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = value[1 : len(value)-1]
			}
			if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("env file is empty")
	}
	return values, nil
}
