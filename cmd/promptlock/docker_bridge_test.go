package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResolveDockerBrokerTransportUsesLocalBridgeOnDarwin(t *testing.T) {
	socketPath := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/meta/capabilities":
			http.NotFound(w, r)
			return
		case "/v1/intents/resolve":
			_, _ = io.WriteString(w, `{"secrets":["github_token"]}`)
			return
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Setenv("PROMPTLOCK_DOCKER_HOST_ALIAS", "127.0.0.1")

	transport, err := resolveDockerBrokerTransport("darwin", brokerSelection{
		BaseURL:    defaultBrokerURL,
		UnixSocket: socketPath,
	}, "/run/promptlock/promptlock-agent.sock")
	if err != nil {
		t.Fatalf("resolveDockerBrokerTransport: %v", err)
	}
	t.Cleanup(func() {
		if err := transport.Close(); err != nil {
			t.Fatalf("close transport: %v", err)
		}
	})

	if transport.BrokerUnixSocket != "" {
		t.Fatalf("expected darwin transport to avoid unix socket mount, got %q", transport.BrokerUnixSocket)
	}
	if !strings.HasPrefix(transport.BrokerURL, "http://127.0.0.1:") {
		t.Fatalf("broker url = %q, want local bridge url", transport.BrokerURL)
	}

	resp, err := http.Get(transport.BrokerURL + "/v1/intents/resolve")
	if err != nil {
		t.Fatalf("bridge request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read bridge response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bridge status = %d, want 200 (body=%q)", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "github_token") {
		t.Fatalf("bridge body = %q, want forwarded payload", string(body))
	}
}

func TestResolveDockerBrokerTransportKeepsUnixSocketOnLinux(t *testing.T) {
	transport, err := resolveDockerBrokerTransport("linux", brokerSelection{
		BaseURL:    defaultBrokerURL,
		UnixSocket: "/tmp/promptlock-agent.sock",
	}, "/run/promptlock/promptlock-agent.sock")
	if err != nil {
		t.Fatalf("resolveDockerBrokerTransport: %v", err)
	}
	t.Cleanup(func() {
		if err := transport.Close(); err != nil {
			t.Fatalf("close transport: %v", err)
		}
	})

	if transport.BrokerUnixSocket != "/tmp/promptlock-agent.sock" {
		t.Fatalf("broker unix socket = %q, want preserved unix socket", transport.BrokerUnixSocket)
	}
	if transport.BrokerURL != defaultBrokerURL {
		t.Fatalf("broker url = %q, want %q", transport.BrokerURL, defaultBrokerURL)
	}
}

func TestResolveDockerBrokerTransportPrefersConfiguredDaemonBridgeURL(t *testing.T) {
	t.Setenv("PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL", "http://host.docker.internal:8766")

	transport, err := resolveDockerBrokerTransport("darwin", brokerSelection{
		BaseURL:    defaultBrokerURL,
		UnixSocket: "/tmp/promptlock-agent.sock",
	}, "/run/promptlock/promptlock-agent.sock")
	if err != nil {
		t.Fatalf("resolveDockerBrokerTransport: %v", err)
	}
	if transport.BrokerURL != "http://host.docker.internal:8766" {
		t.Fatalf("broker url = %q, want configured daemon bridge url", transport.BrokerURL)
	}
	if transport.BrokerUnixSocket != "" {
		t.Fatalf("expected broker unix socket to be omitted when daemon bridge url is configured, got %q", transport.BrokerUnixSocket)
	}
}

func TestResolveDockerBrokerTransportDiscoversDaemonBridgeURLFromCapabilities(t *testing.T) {
	socketPath := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta/capabilities" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"auth_enabled":true,"allow_plaintext_secret_return":false,"agent_bridge_address":"127.0.0.1:8766"}`)
	}))
	t.Setenv("PROMPTLOCK_DOCKER_HOST_ALIAS", "127.0.0.1")

	transport, err := resolveDockerBrokerTransport("darwin", brokerSelection{
		BaseURL:    defaultBrokerURL,
		UnixSocket: socketPath,
	}, "/run/promptlock/promptlock-agent.sock")
	if err != nil {
		t.Fatalf("resolveDockerBrokerTransport: %v", err)
	}
	if transport.BrokerURL != "http://127.0.0.1:8766" {
		t.Fatalf("broker url = %q, want discovered daemon bridge url", transport.BrokerURL)
	}
	if transport.BrokerUnixSocket != "" {
		t.Fatalf("expected broker unix socket to be omitted when daemon advertises bridge, got %q", transport.BrokerUnixSocket)
	}
}
