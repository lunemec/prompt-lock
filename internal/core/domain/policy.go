package domain

import (
	"fmt"
	"strings"
)

type Policy struct {
	DefaultTTLMinutes int
	MinTTLMinutes     int
	MaxTTLMinutes     int
	MaxSecretsPerReq  int
}

func DefaultPolicy() Policy {
	return Policy{
		DefaultTTLMinutes: 5,
		MinTTLMinutes:     1,
		MaxTTLMinutes:     60,
		MaxSecretsPerReq:  5,
	}
}

func (p Policy) ValidateRequest(ttl int, secrets []string) error {
	if ttl < p.MinTTLMinutes || ttl > p.MaxTTLMinutes {
		return fmt.Errorf("ttl_minutes must be in range [%d, %d]", p.MinTTLMinutes, p.MaxTTLMinutes)
	}
	if len(secrets) == 0 {
		return fmt.Errorf("at least one secret required")
	}
	if len(secrets) > p.MaxSecretsPerReq {
		return fmt.Errorf("too many secrets requested (max %d)", p.MaxSecretsPerReq)
	}
	for _, secret := range secrets {
		if err := validateSecretName(secret); err != nil {
			return err
		}
	}
	return nil
}

var reservedSecretEnvNames = map[string]struct{}{
	"PATH":                  {},
	"HOME":                  {},
	"TMPDIR":                {},
	"TMP":                   {},
	"TEMP":                  {},
	"SYSTEMROOT":            {},
	"COMSPEC":               {},
	"PATHEXT":               {},
	"USERPROFILE":           {},
	"LD_PRELOAD":            {},
	"LD_LIBRARY_PATH":       {},
	"DYLD_INSERT_LIBRARIES": {},
	"DYLD_LIBRARY_PATH":     {},
	"PYTHONPATH":            {},
	"PYTHONHOME":            {},
	"GIT_CONFIG":            {},
	"GIT_CONFIG_GLOBAL":     {},
	"GIT_CONFIG_SYSTEM":     {},
	"GIT_EXEC_PATH":         {},
	"NODE_OPTIONS":          {},
	"RUBYOPT":               {},
	"PERL5OPT":              {},
}

func SecretEnvName(secretName string) string {
	trimmed := strings.TrimSpace(secretName)
	if trimmed == "" {
		return ""
	}
	envName := strings.ToUpper(trimmed)
	if !isValidSecretEnvName(envName) {
		return ""
	}
	return envName
}

func validateSecretName(secret string) error {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return fmt.Errorf("secret names must be non-empty")
	}
	envName := SecretEnvName(trimmed)
	if envName == "" {
		return fmt.Errorf("secret name %q must map to a safe environment variable name", trimmed)
	}
	if _, reserved := reservedSecretEnvNames[envName]; reserved {
		return fmt.Errorf("secret name %q is reserved and cannot be leased", trimmed)
	}
	return nil
}

func isValidSecretEnvName(envName string) bool {
	if envName == "" {
		return false
	}
	for i := 0; i < len(envName); i++ {
		c := envName[i]
		switch {
		case c == '_' || (c >= 'A' && c <= 'Z'):
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return envName[0] == '_' || (envName[0] >= 'A' && envName[0] <= 'Z')
}
