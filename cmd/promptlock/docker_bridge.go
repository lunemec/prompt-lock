package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

var dockerRuntimeGOOS = runtime.GOOS

type dockerBrokerTransport struct {
	BrokerURL             string
	BrokerUnixSocket      string
	ContainerBrokerSocket string
	closer                io.Closer
}

func (t dockerBrokerTransport) Close() error {
	if t.closer == nil {
		return nil
	}
	return t.closer.Close()
}

func resolveDockerBrokerTransport(goos string, broker brokerSelection, containerSocket string) (dockerBrokerTransport, error) {
	transport := dockerBrokerTransport{
		BrokerURL:             broker.BaseURL,
		BrokerUnixSocket:      broker.UnixSocket,
		ContainerBrokerSocket: containerSocket,
	}
	if strings.TrimSpace(broker.UnixSocket) == "" || strings.EqualFold(strings.TrimSpace(goos), "linux") {
		return transport, nil
	}
	if bridgeURL := strings.TrimSpace(os.Getenv("PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL")); bridgeURL != "" {
		return dockerBrokerTransport{BrokerURL: bridgeURL}, nil
	}
	if bridgeURL := discoverDaemonDockerBridgeURL(broker, dockerHostAlias()); bridgeURL != "" {
		return dockerBrokerTransport{BrokerURL: bridgeURL}, nil
	}

	bridge, err := startUnixSocketHTTPBridge(broker.UnixSocket, dockerHostAlias())
	if err != nil {
		return dockerBrokerTransport{}, err
	}
	return dockerBrokerTransport{
		BrokerURL: bridge.ContainerURL,
		closer:    bridge,
	}, nil
}

func dockerHostAlias() string {
	if alias := strings.TrimSpace(os.Getenv("PROMPTLOCK_DOCKER_HOST_ALIAS")); alias != "" {
		return alias
	}
	return "host.docker.internal"
}

func discoverDaemonDockerBridgeURL(broker brokerSelection, hostAlias string) string {
	caps, err := brokerCapabilities(broker.BaseURL, broker.UnixSocket)
	if err != nil {
		return ""
	}
	return dockerBridgeURLForHostAlias(caps.AgentBridgeAddress, hostAlias)
}

func dockerBridgeURLForHostAlias(address, hostAlias string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || strings.TrimSpace(port) == "" || strings.TrimSpace(port) == "0" {
		return ""
	}
	alias := strings.TrimSpace(hostAlias)
	if alias == "" {
		return ""
	}
	return fmt.Sprintf("http://%s:%s", alias, port)
}

type unixSocketHTTPBridge struct {
	ContainerURL string

	listener net.Listener
	server   *http.Server
	done     chan struct{}
}

func startUnixSocketHTTPBridge(unixSocket, hostAlias string) (*unixSocketHTTPBridge, error) {
	if strings.TrimSpace(unixSocket) == "" {
		return nil, fmt.Errorf("unix socket path is required for docker bridge")
	}
	target, err := url.Parse(unixSocketRequestBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse unix socket bridge target: %w", err)
	}
	transport := &http.Transport{}
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "unix", unixSocket)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for docker bridge: %w", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("docker bridge listener is not TCP: %T", ln.Addr())
	}
	bridge := &unixSocketHTTPBridge{
		ContainerURL: fmt.Sprintf("http://%s:%d", strings.TrimSpace(hostAlias), addr.Port),
		listener:     ln,
		server:       &http.Server{Handler: proxy},
		done:         make(chan struct{}),
	}
	go func() {
		defer close(bridge.done)
		_ = bridge.server.Serve(ln)
	}()
	return bridge, nil
}

func (b *unixSocketHTTPBridge) Close() error {
	if b == nil || b.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := b.server.Shutdown(ctx)
	<-b.done
	if err == nil || strings.Contains(err.Error(), "Server closed") {
		return nil
	}
	return err
}
