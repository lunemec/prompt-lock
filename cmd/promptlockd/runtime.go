package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/config"
)

func serveConfiguredListeners(ctx context.Context, cfg config.Config, s *server) error {
	switch {
	case cfg.UsesLegacyUnixSocket():
		mux := http.NewServeMux()
		s.registerLegacyRoutesTo(mux)
		return serveUnixSocket(ctx, cfg.UnixSocket, 0o600, mux, "unix socket")
	case cfg.UsesDualUnixSockets():
		if strings.TrimSpace(cfg.OperatorUnixSocket) == "" || strings.TrimSpace(cfg.AgentUnixSocket) == "" {
			return fmt.Errorf("agent_unix_socket and operator_unix_socket are both required in dual-socket mode")
		}
		agentMux := http.NewServeMux()
		s.registerAgentRoutesTo(agentMux)
		operatorMux := http.NewServeMux()
		s.registerOperatorRoutesTo(operatorMux)
		targets, err := newUnixSocketTargets([]unixSocketTarget{
			{Path: cfg.AgentUnixSocket, Mode: 0o660, Handler: agentMux, Label: "agent unix socket"},
			{Path: cfg.OperatorUnixSocket, Mode: 0o600, Handler: operatorMux, Label: "operator unix socket"},
		})
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.AgentBridgeAddress) != "" {
			bridgeTarget, err := newTCPListenerTarget(cfg.AgentBridgeAddress, agentMux, "agent bridge")
			if err != nil {
				return err
			}
			targets = append(targets, bridgeTarget)
			s.agentBridgeAddress = bridgeTarget.Listener.Addr().String()
		}
		return serveListenerTargets(ctx, targets)
	default:
		mux := http.NewServeMux()
		s.registerLegacyRoutesTo(mux)
		target, err := newTCPListenerTarget(cfg.Address, mux, "tcp listener")
		if err != nil {
			return err
		}
		return serveListenerTargets(ctx, []listenerTarget{target})
	}
}

type unixSocketTarget struct {
	Path    string
	Mode    os.FileMode
	Handler http.Handler
	Label   string
}

type listenerTarget struct {
	Listener net.Listener
	Handler  http.Handler
	Label    string
	Cleanup  func()
}

func serveUnixSocket(ctx context.Context, path string, mode os.FileMode, handler http.Handler, label string) error {
	targets, err := newUnixSocketTargets([]unixSocketTarget{{Path: path, Mode: mode, Handler: handler, Label: label}})
	if err != nil {
		return err
	}
	return serveListenerTargets(ctx, targets)
}

func newUnixSocketTargets(targets []unixSocketTarget) ([]listenerTarget, error) {
	out := make([]listenerTarget, 0, len(targets))
	for _, target := range targets {
		target := target
		if err := ensureUnixSocketParentDir(target.Path); err != nil {
			return nil, err
		}
		_ = os.Remove(target.Path)
		ln, err := net.Listen("unix", target.Path)
		if err != nil {
			return nil, err
		}
		if err := os.Chmod(target.Path, target.Mode); err != nil {
			_ = ln.Close()
			_ = os.Remove(target.Path)
			return nil, err
		}
		log.Printf("promptlock listening on %s %s", target.Label, target.Path)
		out = append(out, listenerTarget{
			Listener: ln,
			Handler:  target.Handler,
			Label:    target.Path,
			Cleanup: func() {
				_ = ln.Close()
				_ = os.Remove(target.Path)
			},
		})
	}
	return out, nil
}

func ensureUnixSocketParentDir(path string) error {
	parent := filepath.Dir(strings.TrimSpace(path))
	if parent == "" || parent == "." {
		return nil
	}
	return os.MkdirAll(parent, 0o700)
}

func newTCPListenerTarget(address string, handler http.Handler, label string) (listenerTarget, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return listenerTarget{}, err
	}
	actual := ln.Addr().String()
	log.Printf("promptlock listening on %s %s", label, actual)
	return listenerTarget{
		Listener: ln,
		Handler:  handler,
		Label:    actual,
		Cleanup: func() {
			_ = ln.Close()
		},
	}, nil
}

func serveHTTPListener(ctx context.Context, srv *http.Server, ln net.Listener) error {
	return serveListenerTargets(ctx, []listenerTarget{{
		Listener: ln,
		Handler:  srv.Handler,
		Label:    ln.Addr().String(),
		Cleanup: func() {
			_ = ln.Close()
		},
	}})
}

func serveListenerTargets(ctx context.Context, targets []listenerTarget) error {
	type cleanupFunc func()
	cleanups := make([]cleanupFunc, 0, len(targets))
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()
	servers := make([]*http.Server, 0, len(targets))
	errCh := make(chan error, len(targets))
	for _, target := range targets {
		target := target
		cleanup := target.Cleanup
		if cleanup == nil {
			cleanup = func() {}
		}
		cleanups = append(cleanups, cleanup)
		srv := &http.Server{Handler: target.Handler}
		servers = append(servers, srv)
		go func(srv *http.Server) {
			if err := srv.Serve(target.Listener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
				errCh <- fmt.Errorf("listener %s failed: %w", target.Label, err)
			}
		}(srv)
	}
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var shutdownErr error
		for _, srv := range servers {
			if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) && shutdownErr == nil {
				shutdownErr = err
			}
		}
		return shutdownErr
	}
}
