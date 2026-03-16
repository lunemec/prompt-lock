package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/lunemec/promptlock/internal/config"
)

func isLocalAddress(addr string) bool {
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

func validateSecretSourceSafety(cfg config.Config) error {
	src := strings.ToLower(strings.TrimSpace(cfg.SecretSource.Type))
	if src == "" {
		src = "in_memory"
	}
	if src != "in_memory" && src != "env" && src != "file" && src != "external" {
		return fmt.Errorf("unsupported secret_source.type %q (supported: in_memory, env, file, external)", cfg.SecretSource.Type)
	}
	if src == "file" && strings.TrimSpace(cfg.SecretSource.FilePath) == "" {
		return fmt.Errorf("secret_source.type=file requires secret_source.file_path")
	}
	if src == "external" {
		if strings.TrimSpace(cfg.SecretSource.ExternalURL) == "" {
			return fmt.Errorf("secret_source.type=external requires secret_source.external_url")
		}
		if strings.TrimSpace(cfg.SecretSource.ExternalAuthTokenEnv) == "" {
			return fmt.Errorf("secret_source.type=external requires secret_source.external_auth_token_env")
		}
		u, err := url.Parse(strings.TrimSpace(cfg.SecretSource.ExternalURL))
		if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
			return fmt.Errorf("secret_source.external_url is invalid: %q", cfg.SecretSource.ExternalURL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("secret_source.external_url scheme must be http or https")
		}
	}
	if strings.ToLower(strings.TrimSpace(cfg.SecurityProfile)) == "hardened" && src == "in_memory" {
		mode := strings.ToLower(strings.TrimSpace(cfg.SecretSource.InMemoryHardened))
		if mode == "fail" {
			return fmt.Errorf("hardened profile with in_memory secret_source is disallowed (secret_source.in_memory_hardened=fail)")
		}
	}
	return nil
}

func normalizedStateStoreType(cfg config.Config) string {
	mode := strings.ToLower(strings.TrimSpace(cfg.StateStore.Type))
	if mode == "" {
		return "file"
	}
	return mode
}

func validateStateStoreSafety(cfg config.Config) error {
	mode := normalizedStateStoreType(cfg)
	switch mode {
	case "file":
		return nil
	case "external":
		if strings.TrimSpace(cfg.StateStore.ExternalURL) == "" {
			return fmt.Errorf("state_store.type=external requires state_store.external_url")
		}
		if strings.TrimSpace(cfg.StateStore.ExternalAuthTokenEnv) == "" {
			return fmt.Errorf("state_store.type=external requires state_store.external_auth_token_env")
		}
		u, err := url.Parse(strings.TrimSpace(cfg.StateStore.ExternalURL))
		if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
			return fmt.Errorf("state_store.external_url is invalid: %q", cfg.StateStore.ExternalURL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("state_store.external_url scheme must be http or https")
		}
		return nil
	default:
		return fmt.Errorf("unsupported state_store.type %q (supported: file, external)", cfg.StateStore.Type)
	}
}

func validateSocketConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.UnixSocket) != "" && (strings.TrimSpace(cfg.AgentUnixSocket) != "" || strings.TrimSpace(cfg.OperatorUnixSocket) != "") {
		return fmt.Errorf("legacy unix_socket cannot be combined with agent_unix_socket or operator_unix_socket")
	}
	agentPath := strings.TrimSpace(cfg.AgentUnixSocket)
	operatorPath := strings.TrimSpace(cfg.OperatorUnixSocket)
	if agentPath != "" && operatorPath != "" && agentPath == operatorPath {
		return fmt.Errorf("agent_unix_socket and operator_unix_socket must be different paths")
	}
	return nil
}

func validateTransportSafety(cfg config.Config, allowInsecureAuthTCP, allowInsecureNoAuthTCP string) error {
	if cfg.Auth.EnableAuth && !cfg.UsesUnixSocketTransport() && !isLocalAddress(cfg.Address) && allowInsecureAuthTCP != "1" {
		return fmt.Errorf("auth enabled on non-local TCP without unix socket transport; set unix_socket or agent_unix_socket/operator_unix_socket, or PROMPTLOCK_ALLOW_INSECURE_TCP=1")
	}
	if !cfg.Auth.EnableAuth && !cfg.UsesUnixSocketTransport() && !isLocalAddress(cfg.Address) && allowInsecureNoAuthTCP != "1" {
		return fmt.Errorf("auth disabled on non-local TCP without unix socket transport; enable auth, set unix_socket or agent_unix_socket/operator_unix_socket, or PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1")
	}
	return nil
}

func isInsecureDevMode(cfg config.Config) bool {
	return !cfg.Auth.EnableAuth && cfg.Auth.AllowPlaintextSecretReturn
}

func validateSecurityProfile(cfg config.Config, allowInsecureProfile string) error {
	profile := strings.TrimSpace(strings.ToLower(cfg.SecurityProfile))
	if profile == "" {
		profile = "dev"
	}
	if profile == "dev" {
		return nil
	}
	if profile == "insecure" && allowInsecureProfile != "1" {
		return fmt.Errorf("security_profile=insecure requires explicit opt-in: set PROMPTLOCK_ALLOW_INSECURE_PROFILE=1")
	}
	if profile != "dev" && !cfg.Auth.EnableAuth {
		return fmt.Errorf("security_profile=%s requires auth.enable_auth=true", profile)
	}
	return nil
}

func validateDeploymentMode(cfg config.Config, allowDevProfile string) error {
	profile := strings.TrimSpace(strings.ToLower(cfg.SecurityProfile))
	if profile == "" {
		profile = "dev"
	}
	if profile == "dev" && allowDevProfile != "1" {
		return fmt.Errorf("security_profile=dev is disabled by default; set PROMPTLOCK_ALLOW_DEV_PROFILE=1 for local testing or use security_profile=hardened")
	}
	if profile != "dev" {
		mode := normalizedStateStoreType(cfg)
		switch mode {
		case "file":
			if strings.TrimSpace(cfg.StateStoreFile) == "" {
				return fmt.Errorf("non-dev profile requires state_store_file for durable request/lease state when state_store.type=file")
			}
		case "external":
			extURL := strings.TrimSpace(cfg.StateStore.ExternalURL)
			if extURL == "" {
				return fmt.Errorf("non-dev profile with state_store.type=external requires state_store.external_url")
			}
			u, err := url.Parse(extURL)
			if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
				return fmt.Errorf("state_store.external_url is invalid: %q", cfg.StateStore.ExternalURL)
			}
			if u.Scheme != "https" {
				return fmt.Errorf("non-dev profile requires https state_store.external_url when state_store.type=external")
			}
			tokenEnv := strings.TrimSpace(cfg.StateStore.ExternalAuthTokenEnv)
			if tokenEnv == "" {
				return fmt.Errorf("non-dev profile with state_store.type=external requires state_store.external_auth_token_env")
			}
			if strings.TrimSpace(os.Getenv(tokenEnv)) == "" {
				return fmt.Errorf("state_store.type=external requires auth token env %s to be set in %s profile", tokenEnv, profile)
			}
		default:
			return fmt.Errorf("unsupported state_store.type %q (supported: file, external)", cfg.StateStore.Type)
		}
		if strings.TrimSpace(cfg.Auth.StoreFile) == "" {
			return fmt.Errorf("non-dev profile requires auth.store_file for durable auth state")
		}
		src := strings.ToLower(strings.TrimSpace(cfg.SecretSource.Type))
		if src == "" {
			src = "in_memory"
		}
		if src == "in_memory" {
			return fmt.Errorf("non-dev profile requires secret_source.type of env, file, or external (in_memory is not allowed)")
		}
		if src == "external" {
			extURL := strings.TrimSpace(cfg.SecretSource.ExternalURL)
			if extURL == "" {
				return fmt.Errorf("non-dev profile with secret_source.type=external requires secret_source.external_url")
			}
			u, err := url.Parse(extURL)
			if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
				return fmt.Errorf("secret_source.external_url is invalid: %q", cfg.SecretSource.ExternalURL)
			}
			if u.Scheme != "https" {
				return fmt.Errorf("non-dev profile requires https secret_source.external_url when secret_source.type=external")
			}
			tokenEnv := strings.TrimSpace(cfg.SecretSource.ExternalAuthTokenEnv)
			if tokenEnv == "" {
				return fmt.Errorf("non-dev profile with secret_source.type=external requires secret_source.external_auth_token_env")
			}
			if strings.TrimSpace(os.Getenv(tokenEnv)) == "" {
				return fmt.Errorf("secret_source.type=external requires auth token env %s to be set in %s profile", tokenEnv, profile)
			}
		}
	}
	if profile != "dev" && cfg.Auth.EnableAuth && strings.TrimSpace(cfg.Auth.StoreFile) != "" {
		keyEnv := strings.TrimSpace(cfg.Auth.StoreEncryptionKeyEnv)
		if keyEnv == "" {
			keyEnv = "PROMPTLOCK_AUTH_STORE_KEY"
		}
		if strings.TrimSpace(os.Getenv(keyEnv)) == "" {
			return fmt.Errorf("auth.store_file requires encrypted persistence in %s profile; set %s", profile, keyEnv)
		}
	}
	return nil
}

func resolveAuthStoreEncryptionKey(cfg config.Config) ([]byte, error) {
	keyEnv := strings.TrimSpace(cfg.Auth.StoreEncryptionKeyEnv)
	if keyEnv == "" {
		keyEnv = "PROMPTLOCK_AUTH_STORE_KEY"
	}
	v := strings.TrimSpace(os.Getenv(keyEnv))
	if v == "" {
		return nil, nil
	}
	if len(v) < 16 {
		return nil, fmt.Errorf("%s must be at least 16 characters", keyEnv)
	}
	return []byte(v), nil
}
