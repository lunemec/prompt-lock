package sopsenv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	DefaultEnvFileEnv = "PROMPTLOCK_SOPS_ENV_FILE"
	decryptTimeout    = 15 * time.Second
)

var decryptFile = decryptWithSOPS

// LoadFromFile decrypts a SOPS file and loads key/value pairs into process env.
// Existing environment variables are preserved (explicit env always wins).
func LoadFromFile(path string, requiredKeys []string) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil
	}
	decrypted, err := decryptFile(trimmedPath)
	if err != nil {
		return err
	}
	values, err := parseDecryptedPayload(decrypted)
	if err != nil {
		return fmt.Errorf("parse decrypted sops env %s: %w", trimmedPath, err)
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s from sops file %s: %w", key, trimmedPath, err)
		}
	}
	for _, required := range requiredKeys {
		requiredKey := strings.TrimSpace(required)
		if requiredKey == "" {
			continue
		}
		if strings.TrimSpace(os.Getenv(requiredKey)) == "" {
			return fmt.Errorf("required env %s missing after loading sops file %s", requiredKey, trimmedPath)
		}
	}
	return nil
}

func decryptWithSOPS(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), decryptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sops", "--decrypt", path)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("decrypt sops file %s: timed out after %s", path, decryptTimeout)
	}
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return nil, fmt.Errorf("decrypt sops file %s: %v: %s", path, err, detail)
		}
		return nil, fmt.Errorf("decrypt sops file %s: %w", path, err)
	}
	return out, nil
}

func parseDecryptedPayload(data []byte) (map[string]string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("decrypted payload is empty")
	}
	if trimmed[0] == '{' {
		var raw map[string]any
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return nil, fmt.Errorf("json object decode failed: %w", err)
		}
		if len(raw) == 0 {
			return nil, fmt.Errorf("json object payload is empty")
		}
		out := make(map[string]string, len(raw))
		for key, value := range raw {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, fmt.Errorf("json payload contains empty env key")
			}
			stringValue, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("json payload value for %s must be string", trimmedKey)
			}
			out[trimmedKey] = stringValue
		}
		return out, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	out := map[string]string{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
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
		out[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read dotenv payload: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("dotenv payload is empty")
	}
	return out, nil
}
