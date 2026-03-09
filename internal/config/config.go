package config

import (
	"encoding/json"
	"net"
	"os"
	"strings"

	"github.com/lunemec/promptlock/internal/core/domain"
)

type Config struct {
	SecurityProfile     string              `json:"security_profile"`
	Address             string              `json:"address"`
	UnixSocket          string              `json:"unix_socket"`
	AuditPath           string              `json:"audit_path"`
	StateStoreFile      string              `json:"state_store_file"`
	Policy              PolicyConfig        `json:"policy"`
	Auth                AuthConfig          `json:"auth"`
	ExecutionPolicy     ExecutionPolicy     `json:"execution_policy"`
	HostOpsPolicy       HostOpsPolicy       `json:"host_ops_policy"`
	NetworkEgressPolicy NetworkEgressPolicy `json:"network_egress_policy"`
	TLS                 TLSConfig           `json:"tls"`
	SecretSource        SecretSourceConfig  `json:"secret_source"`
	Secrets             []SecretEntry       `json:"secrets"`
	Intents             IntentMap           `json:"intents"`
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
		SecurityProfile:     "dev",
		Address:             "127.0.0.1:8765",
		UnixSocket:          "",
		AuditPath:           "/tmp/promptlock-audit.jsonl",
		Intents:             IntentMap{},
		Auth:                defaultAuthConfig(),
		ExecutionPolicy:     defaultExecutionPolicy(),
		HostOpsPolicy:       defaultHostOpsPolicy(),
		NetworkEgressPolicy: defaultNetworkEgressPolicy(),
		TLS:                 defaultTLSConfig(),
		SecretSource:        defaultSecretSourceConfig(),
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
	cfg.applyProfile()
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	switch c.ExecutionPolicy.OutputSecurityMode {
	case "", "redacted", "raw", "none":
		if c.ExecutionPolicy.OutputSecurityMode == "" {
			c.ExecutionPolicy.OutputSecurityMode = "redacted"
		}
	default:
		c.ExecutionPolicy.OutputSecurityMode = "redacted"
	}
	if c.SecretSource.Type == "" {
		c.SecretSource.Type = "in_memory"
	}
	if strings.TrimSpace(c.Auth.StoreEncryptionKeyEnv) == "" {
		c.Auth.StoreEncryptionKeyEnv = "PROMPTLOCK_AUTH_STORE_KEY"
	}
	if c.SecretSource.EnvPrefix == "" {
		c.SecretSource.EnvPrefix = "PROMPTLOCK_SECRET_"
	}
	if c.SecretSource.Type == "file" && c.SecretSource.FilePath == "" {
		c.SecretSource.FilePath = "/etc/promptlock/secrets.json"
	}
	if strings.TrimSpace(c.SecretSource.ExternalAuthTokenEnv) == "" {
		c.SecretSource.ExternalAuthTokenEnv = "PROMPTLOCK_EXTERNAL_SECRET_TOKEN"
	}
	if c.SecretSource.ExternalTimeoutSec <= 0 {
		c.SecretSource.ExternalTimeoutSec = 10
	}
	switch c.SecretSource.InMemoryHardened {
	case "", "warn", "fail":
		if c.SecretSource.InMemoryHardened == "" {
			c.SecretSource.InMemoryHardened = "warn"
		}
	default:
		c.SecretSource.InMemoryHardened = "warn"
	}
}

func (c *Config) applyProfile() {
	switch c.SecurityProfile {
	case "", "dev":
		return
	case "hardened":
		c.Auth.AllowPlaintextSecretReturn = false
		c.NetworkEgressPolicy.RequireIntentMatch = true
		if c.ExecutionPolicy.MaxTimeoutSec > 120 {
			c.ExecutionPolicy.MaxTimeoutSec = 120
		}
		if c.ExecutionPolicy.DefaultTimeoutSec > 60 {
			c.ExecutionPolicy.DefaultTimeoutSec = 60
		}
		c.ExecutionPolicy.OutputSecurityMode = "none"
		if c.ExecutionPolicy.MaxOutputBytes > 32768 {
			c.ExecutionPolicy.MaxOutputBytes = 32768
		}
		c.ExecutionPolicy.AllowlistPrefixes = []string{"npm", "node", "go", "python", "pytest", "make", "git"}
		c.ExecutionPolicy.DenylistSubstrings = append(c.ExecutionPolicy.DenylistSubstrings,
			"&&", "||", ";", "$(", "`")
		c.HostOpsPolicy.DockerComposeAllowVerbs = []string{"config", "ps"}
		c.HostOpsPolicy.DockerDenySubstrings = append(c.HostOpsPolicy.DockerDenySubstrings,
			"&&", "||", ";", "$(", "`")
		if c.UnixSocket == "" && !c.TLS.Enable && isLocalAddressConfig(c.Address) {
			c.UnixSocket = "/tmp/promptlock.sock"
		}
	default:
		return
	}
}

func isLocalAddressConfig(addr string) bool {
	a := strings.TrimSpace(addr)
	if a == "" {
		return false
	}
	host := a
	if h, _, err := net.SplitHostPort(a); err == nil {
		host = h
	} else if strings.HasPrefix(a, "[") && strings.HasSuffix(a, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(a, "["), "]")
	}
	host = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")))
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func (c Config) ToPolicy() domain.Policy {
	return domain.Policy{
		DefaultTTLMinutes: c.Policy.DefaultTTLMinutes,
		MinTTLMinutes:     c.Policy.MinTTLMinutes,
		MaxTTLMinutes:     c.Policy.MaxTTLMinutes,
		MaxSecretsPerReq:  c.Policy.MaxSecretsPerReq,
	}
}
