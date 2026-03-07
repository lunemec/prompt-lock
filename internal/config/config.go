package config

import (
	"encoding/json"
	"os"

	"github.com/lunemec/promptlock/internal/core/domain"
)

type Config struct {
	Address         string          `json:"address"`
	UnixSocket      string          `json:"unix_socket"`
	AuditPath       string          `json:"audit_path"`
	Policy          PolicyConfig    `json:"policy"`
	Auth            AuthConfig      `json:"auth"`
	ExecutionPolicy ExecutionPolicy `json:"execution_policy"`
	Secrets         []SecretEntry   `json:"secrets"`
	Intents         IntentMap       `json:"intents"`
}

type PolicyConfig struct {
	DefaultTTLMinutes int `json:"default_ttl_minutes"`
	MinTTLMinutes     int `json:"min_ttl_minutes"`
	MaxTTLMinutes     int `json:"max_ttl_minutes"`
	MaxSecretsPerReq  int `json:"max_secrets_per_request"`
}

type SecretEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func Default() Config {
	p := domain.DefaultPolicy()
	return Config{
		Address:         "127.0.0.1:8765",
		UnixSocket:      "",
		AuditPath:       "/tmp/promptlock-audit.jsonl",
		Intents:         IntentMap{},
		Auth:            defaultAuthConfig(),
		ExecutionPolicy: defaultExecutionPolicy(),
		Policy: PolicyConfig{
			DefaultTTLMinutes: p.DefaultTTLMinutes,
			MinTTLMinutes:     p.MinTTLMinutes,
			MaxTTLMinutes:     p.MaxTTLMinutes,
			MaxSecretsPerReq:  p.MaxSecretsPerReq,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, err
	}
	if len(b) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) ToPolicy() domain.Policy {
	return domain.Policy{
		DefaultTTLMinutes: c.Policy.DefaultTTLMinutes,
		MinTTLMinutes:     c.Policy.MinTTLMinutes,
		MaxTTLMinutes:     c.Policy.MaxTTLMinutes,
		MaxSecretsPerReq:  c.Policy.MaxSecretsPerReq,
	}
}
