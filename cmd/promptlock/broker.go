package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/config"
)

type capabilities struct {
	AuthEnabled                bool `json:"auth_enabled"`
	AllowPlaintextSecretReturn bool `json:"allow_plaintext_secret_return"`
}

type brokerFlags struct {
	Broker     *string
	BrokerUnix *string
}

var doPostJSONAuth = postJSONAuth
var brokerClientTimeout = 10 * time.Second

const (
	defaultBrokerURL         = "http://127.0.0.1:8765"
	unixSocketRequestBaseURL = "http://promptlock"
	watchAllowUsageText      = "usage: promptlock watch allow [--broker URL | --broker-unix-socket PATH] [--ttl N] <request_id>"
	watchDenyUsageText       = "usage: promptlock watch deny [--broker URL | --broker-unix-socket PATH] [--reason TEXT] <request_id>"
	containerSocketHelpText  = "path inside the container where the selected agent broker unix socket is mounted when agent transport uses a unix socket; ignored for TCP --broker/PROMPTLOCK_BROKER_URL"
)

type brokerRole string

const (
	brokerRoleAgent    brokerRole = "agent"
	brokerRoleOperator brokerRole = "operator"
)

type brokerSelectionInput struct {
	BaseURL    string
	UnixSocket string
}

type brokerSelection struct {
	BaseURL    string
	UnixSocket string
}

func registerBrokerFlags(fs *flag.FlagSet) brokerFlags {
	return brokerFlags{
		Broker:     fs.String("broker", "", "broker URL"),
		BrokerUnix: fs.String("broker-unix-socket", "", "broker unix socket path"),
	}
}

func (f brokerFlags) resolve(role brokerRole) (brokerSelection, error) {
	return resolveBrokerSelection(role, brokerSelectionInput{
		BaseURL:    strings.TrimSpace(*f.Broker),
		UnixSocket: strings.TrimSpace(*f.BrokerUnix),
	})
}

func resolveBrokerSelection(role brokerRole, in brokerSelectionInput) (brokerSelection, error) {
	explicitURL := strings.TrimSpace(in.BaseURL)
	explicitUnix := strings.TrimSpace(in.UnixSocket)
	if explicitUnix != "" {
		ready, err := brokerSocketExists(explicitUnix)
		if err != nil {
			if errors.Is(err, errBrokerUnixSocketNotSocket) {
				return brokerSelection{}, fmt.Errorf("broker unix socket path %s is not a unix socket; provide an existing --broker-unix-socket path or use --broker for explicit TCP transport", explicitUnix)
			}
			return brokerSelection{}, fmt.Errorf("validate broker unix socket path %s: %w", explicitUnix, err)
		}
		if !ready {
			return brokerSelection{}, fmt.Errorf("broker unix socket not found at %s; provide an existing --broker-unix-socket path or use --broker for explicit TCP transport", explicitUnix)
		}
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: explicitUnix,
		}, nil
	}
	if explicitURL != "" {
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: "",
		}, nil
	}
	if compatUnix := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_UNIX_SOCKET")); compatUnix != "" {
		ready, err := brokerSocketExists(compatUnix)
		if err != nil {
			if errors.Is(err, errBrokerUnixSocketNotSocket) {
				return brokerSelection{}, fmt.Errorf("compat broker unix socket path %s is not a unix socket; unset PROMPTLOCK_BROKER_UNIX_SOCKET or provide --broker-unix-socket/--broker for an explicit transport", compatUnix)
			}
			return brokerSelection{}, fmt.Errorf("validate compat broker unix socket path %s: %w", compatUnix, err)
		}
		if !ready {
			return brokerSelection{}, fmt.Errorf("compat broker unix socket not found at %s; unset PROMPTLOCK_BROKER_UNIX_SOCKET or provide --broker-unix-socket/--broker for an explicit transport", compatUnix)
		}
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: compatUnix,
		}, nil
	}
	if roleUnix := roleSpecificBrokerUnixSocket(role); roleUnix != "" {
		ready, err := brokerSocketExists(roleUnix)
		if err != nil {
			if errors.Is(err, errBrokerUnixSocketNotSocket) {
				return brokerSelection{}, fmt.Errorf("%s broker unix socket path %s is not a unix socket; set --broker or PROMPTLOCK_BROKER_URL for explicit TCP transport", role, roleUnix)
			}
			return brokerSelection{}, fmt.Errorf("validate %s broker unix socket path %s: %w", role, roleUnix, err)
		}
		if ready {
			return brokerSelection{
				BaseURL:    normalizeBrokerURL(explicitURL),
				UnixSocket: roleUnix,
			}, nil
		}
		if envURL := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_URL")); envURL != "" {
			return brokerSelection{
				BaseURL:    envURL,
				UnixSocket: "",
			}, nil
		}
		return brokerSelection{}, fmt.Errorf("%s broker unix socket not found at %s; set --broker or PROMPTLOCK_BROKER_URL for explicit TCP transport", role, roleUnix)
	}
	defaultUnix := defaultBrokerUnixSocket(role)
	ready, err := brokerSocketExists(defaultUnix)
	if err != nil {
		if errors.Is(err, errBrokerUnixSocketNotSocket) {
			return brokerSelection{}, fmt.Errorf("%s broker unix socket path %s is not a unix socket; set --broker or PROMPTLOCK_BROKER_URL for explicit TCP transport", role, defaultUnix)
		}
		return brokerSelection{}, fmt.Errorf("validate %s broker unix socket path %s: %w", role, defaultUnix, err)
	}
	if ready {
		return brokerSelection{
			BaseURL:    normalizeBrokerURL(explicitURL),
			UnixSocket: defaultUnix,
		}, nil
	}
	if envURL := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_URL")); envURL != "" {
		return brokerSelection{
			BaseURL:    envURL,
			UnixSocket: "",
		}, nil
	}
	return brokerSelection{}, fmt.Errorf("%s broker unix socket not found at %s; set --broker or PROMPTLOCK_BROKER_URL for explicit TCP transport", role, defaultUnix)
}

func normalizeBrokerURL(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if envURL := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_URL")); envURL != "" {
		return envURL
	}
	return defaultBrokerURL
}

func roleSpecificBrokerUnixSocket(role brokerRole) string {
	switch role {
	case brokerRoleOperator:
		return strings.TrimSpace(os.Getenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET"))
	case brokerRoleAgent:
		return strings.TrimSpace(os.Getenv("PROMPTLOCK_AGENT_UNIX_SOCKET"))
	default:
		return ""
	}
}

func defaultBrokerUnixSocket(role brokerRole) string {
	switch role {
	case brokerRoleOperator:
		return config.DefaultOperatorUnixSocketPath
	case brokerRoleAgent:
		return config.DefaultAgentUnixSocketPath
	default:
		return ""
	}
}

var errBrokerUnixSocketNotSocket = errors.New("broker unix socket path is not a unix socket")

func brokerSocketExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return false, errBrokerUnixSocketNotSocket
	}
	return true, nil
}

func watchAllowUsage() string {
	return watchAllowUsageText
}

func watchDenyUsage() string {
	return watchDenyUsageText
}

func authDockerRunContainerBrokerSocketHelp() string {
	return containerSocketHelpText
}

func postJSONAuth(baseURL, unixSocket, path, bearer string, in any, out any) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequest(http.MethodPost, buildURL(baseURL, unixSocket, path), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client, err := httpClient(baseURL, unixSocket)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return normalizeBrokerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return responseError("request failed", resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getAuth(baseURL, unixSocket, path, bearer string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, buildURL(baseURL, unixSocket, path), nil)
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client, err := httpClient(baseURL, unixSocket)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, normalizeBrokerRequestError(err)
	}
	return resp, nil
}

func buildURL(baseURL, unixSocket, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.TrimSpace(unixSocket) != "" {
		baseURL = unixSocketRequestBaseURL
	}
	return strings.TrimRight(baseURL, "/") + path
}

func httpClient(baseURL, unixSocket string) (*http.Client, error) {
	if unixSocket != "" {
		tr := &http.Transport{}
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", unixSocket)
		}
		return &http.Client{Transport: tr, Timeout: brokerClientTimeout}, nil
	}
	return &http.Client{Timeout: brokerClientTimeout}, nil
}

func normalizeBrokerRequestError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("broker request timed out after %s", brokerClientTimeout)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("broker request timed out after %s", brokerClientTimeout)
	}
	return err
}

func responseError(prefix string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("%s: %s", prefix, resp.Status)
	}
	return fmt.Errorf("%s: %s (%s)", prefix, msg, resp.Status)
}
