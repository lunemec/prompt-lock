package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lunemec/promptlock/internal/core/domain"
)

type Config struct {
	SecurityProfile     string              `json:"security_profile"`
	Address             string              `json:"address"`
	UnixSocket          string              `json:"unix_socket"`
	AgentUnixSocket     string              `json:"agent_unix_socket"`
	OperatorUnixSocket  string              `json:"operator_unix_socket"`
	AgentBridgeAddress  string              `json:"agent_bridge_address"`
	AuditPath           string              `json:"audit_path"`
	StateStoreFile      string              `json:"state_store_file"`
	StateStore          StateStoreConfig    `json:"state_store"`
	Policy              PolicyConfig        `json:"policy"`
	RequestPolicy       RequestPolicyConfig `json:"request_policy"`
	Auth                AuthConfig          `json:"auth"`
	ExecutionPolicy     ExecutionPolicy     `json:"execution_policy"`
	HostOpsPolicy       HostOpsPolicy       `json:"host_ops_policy"`
	NetworkEgressPolicy NetworkEgressPolicy `json:"network_egress_policy"`
	SecretSource        SecretSourceConfig  `json:"secret_source"`
	Secrets             []SecretEntry       `json:"secrets"`
	Intents             IntentMap           `json:"intents"`

	executionPolicyExactExecutablesConfigured bool
	executionPolicyOutputModeConfigured       bool
	agentBridgeAddressConfigured              bool
}

const (
	DefaultAgentUnixSocketPath    = "/tmp/promptlock-agent.sock"
	DefaultOperatorUnixSocketPath = "/tmp/promptlock-operator.sock"
	DefaultAgentBridgeAddress     = "127.0.0.1:0"
)

var configRuntimeGOOS = runtime.GOOS

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
		AgentUnixSocket:     "",
		OperatorUnixSocket:  "",
		AgentBridgeAddress:  "",
		AuditPath:           "/tmp/promptlock-audit.jsonl",
		Intents:             IntentMap{},
		Auth:                defaultAuthConfig(),
		StateStore:          defaultStateStoreConfig(),
		ExecutionPolicy:     defaultExecutionPolicy(),
		HostOpsPolicy:       defaultHostOpsPolicy(),
		NetworkEgressPolicy: defaultNetworkEgressPolicy(),
		RequestPolicy:       defaultRequestPolicyConfig(),
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
	if err := rejectRemovedTransportConfig(b); err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	cfg.markExplicitExecutionPolicyOverrides(b)
	cfg.resolveExecutionPolicyCompatibility()
	cfg.applyProfile()
	cfg.normalize()
	return cfg, nil
}

func (c *Config) markExplicitExecutionPolicyOverrides(raw []byte) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return
	}
	execRaw, ok := root["execution_policy"]
	if !ok {
		return
	}
	var exec map[string]json.RawMessage
	if err := json.Unmarshal(execRaw, &exec); err != nil {
		return
	}
	if _, ok := exec["exact_match_executables"]; ok {
		c.executionPolicyExactExecutablesConfigured = true
	}
	if _, ok := exec["output_security_mode"]; ok {
		c.executionPolicyOutputModeConfigured = true
	}
	if _, ok := root["agent_bridge_address"]; ok {
		c.agentBridgeAddressConfigured = true
	}
}

func rejectRemovedTransportConfig(raw []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	if _, ok := root["tls"]; ok {
		return fmt.Errorf("tls transport settings are no longer supported; use unix_socket or agent_unix_socket/operator_unix_socket")
	}
	return nil
}

func (c *Config) resolveExecutionPolicyCompatibility() {}

func (c *Config) normalize() {
	c.AgentBridgeAddress = strings.TrimSpace(c.AgentBridgeAddress)
	c.ExecutionPolicy.ExactMatchExecutables = normalizeStringList(c.ExecutionPolicy.ExactMatchExecutables)
	c.ExecutionPolicy.CommandSearchPaths = normalizePathList(c.ExecutionPolicy.CommandSearchPaths)
	if len(c.ExecutionPolicy.CommandSearchPaths) == 0 {
		c.ExecutionPolicy.CommandSearchPaths = defaultCommandSearchPaths()
	}
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
	if c.StateStore.Type == "" {
		c.StateStore.Type = "file"
	}
	if strings.TrimSpace(c.StateStore.ExternalAuthTokenEnv) == "" {
		c.StateStore.ExternalAuthTokenEnv = "PROMPTLOCK_EXTERNAL_STATE_TOKEN"
	}
	if c.StateStore.ExternalTimeoutSec <= 0 {
		c.StateStore.ExternalTimeoutSec = 10
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
	if c.RequestPolicy.IdenticalRequestCooldownSeconds <= 0 {
		c.RequestPolicy.IdenticalRequestCooldownSeconds = 60
	}
	if c.RequestPolicy.MaxPendingPerAgent <= 0 {
		c.RequestPolicy.MaxPendingPerAgent = 2
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
		if !c.executionPolicyOutputModeConfigured {
			c.ExecutionPolicy.OutputSecurityMode = "none"
		}
		if c.ExecutionPolicy.MaxOutputBytes > 32768 {
			c.ExecutionPolicy.MaxOutputBytes = 32768
		}
		hardenedAllowlist := []string{"npm", "node", "go", "python", "pytest", "make", "git"}
		if c.executionPolicyExecutablesConfigured() {
			c.ExecutionPolicy.ExactMatchExecutables = mergeUniqueStrings(hardenedAllowlist, c.ExecutionPolicy.ExactMatchExecutables)
		} else {
			c.ExecutionPolicy.ExactMatchExecutables = hardenedAllowlist
		}
		c.ExecutionPolicy.ExactMatchExecutables = filterDisallowedShells(c.ExecutionPolicy.ExactMatchExecutables)
		c.ExecutionPolicy.DenylistSubstrings = append(c.ExecutionPolicy.DenylistSubstrings,
			"&&", "||", ";", "$(", "`")
		c.HostOpsPolicy.DockerComposeAllowVerbs = []string{"config", "ps"}
		c.HostOpsPolicy.DockerDenySubstrings = append(c.HostOpsPolicy.DockerDenySubstrings,
			"&&", "||", ";", "$(", "`")
		if !c.hasAnyUnixSocketConfigured() && isLocalAddressConfig(c.Address) {
			c.AgentUnixSocket = DefaultAgentUnixSocketPath
			c.OperatorUnixSocket = DefaultOperatorUnixSocketPath
		}
		if !c.agentBridgeAddressConfigured && c.UsesDualUnixSockets() {
			c.AgentBridgeAddress = DefaultAgentBridgeAddressForGOOS(configRuntimeGOOS)
		}
	default:
		return
	}
}

func (c Config) executionPolicyExecutablesConfigured() bool {
	return c.executionPolicyExactExecutablesConfigured
}

func (c Config) hasAnyUnixSocketConfigured() bool {
	return strings.TrimSpace(c.UnixSocket) != "" || strings.TrimSpace(c.AgentUnixSocket) != "" || strings.TrimSpace(c.OperatorUnixSocket) != ""
}

func (c Config) UsesUnixSocketTransport() bool {
	return c.hasAnyUnixSocketConfigured()
}

func (c Config) UsesLegacyUnixSocket() bool {
	return strings.TrimSpace(c.UnixSocket) != "" && strings.TrimSpace(c.AgentUnixSocket) == "" && strings.TrimSpace(c.OperatorUnixSocket) == ""
}

func (c Config) UsesDualUnixSockets() bool {
	return strings.TrimSpace(c.AgentUnixSocket) != "" || strings.TrimSpace(c.OperatorUnixSocket) != ""
}

func (c Config) UsesAgentBridge() bool {
	return strings.TrimSpace(c.AgentBridgeAddress) != ""
}

func DefaultAgentBridgeAddressForGOOS(goos string) string {
	if strings.EqualFold(strings.TrimSpace(goos), "linux") {
		return ""
	}
	return DefaultAgentBridgeAddress
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

func mergeUniqueStrings(base []string, extras []string) []string {
	out := append([]string{}, base...)
	seen := map[string]struct{}{}
	for _, item := range out {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	for _, item := range extras {
		trimmed := strings.TrimSpace(item)
		key := strings.ToLower(trimmed)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	return out
}

func normalizeStringList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		key := strings.ToLower(trimmed)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	return out
}

func normalizePathList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		cleaned := filepath.Clean(trimmed)
		key := strings.ToLower(cleaned)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, cleaned)
		seen[key] = struct{}{}
	}
	return out
}

func filterDisallowedShells(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		switch strings.ToLower(trimmed) {
		case "", "bash", "sh", "zsh":
			continue
		default:
			out = append(out, trimmed)
		}
	}
	return out
}

func (c Config) ToPolicy() domain.Policy {
	return domain.Policy{
		DefaultTTLMinutes: c.Policy.DefaultTTLMinutes,
		MinTTLMinutes:     c.Policy.MinTTLMinutes,
		MaxTTLMinutes:     c.Policy.MaxTTLMinutes,
		MaxSecretsPerReq:  c.Policy.MaxSecretsPerReq,
	}
}
