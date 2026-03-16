package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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
		return serveUnixSockets(ctx, []unixSocketTarget{
			{Path: cfg.AgentUnixSocket, Mode: 0o660, Handler: agentMux, Label: "agent unix socket"},
			{Path: cfg.OperatorUnixSocket, Mode: 0o600, Handler: operatorMux, Label: "operator unix socket"},
		})
	default:
		mux := http.NewServeMux()
		s.registerLegacyRoutesTo(mux)
		ln, err := net.Listen("tcp", cfg.Address)
		if err != nil {
			return err
		}
		srv := &http.Server{Handler: mux}
		log.Printf("promptlock listening on %s", cfg.Address)
		return serveHTTPListener(ctx, srv, ln)
	}
}

type unixSocketTarget struct {
	Path    string
	Mode    os.FileMode
	Handler http.Handler
	Label   string
}

func serveUnixSocket(ctx context.Context, path string, mode os.FileMode, handler http.Handler, label string) error {
	return serveUnixSockets(ctx, []unixSocketTarget{{Path: path, Mode: mode, Handler: handler, Label: label}})
}

func serveUnixSockets(ctx context.Context, targets []unixSocketTarget) error {
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
		_ = os.Remove(target.Path)
		ln, err := net.Listen("unix", target.Path)
		if err != nil {
			return err
		}
		if err := os.Chmod(target.Path, target.Mode); err != nil {
			_ = ln.Close()
			_ = os.Remove(target.Path)
			return err
		}
		cleanups = append(cleanups, func() {
			_ = ln.Close()
			_ = os.Remove(target.Path)
		})
		log.Printf("promptlock listening on %s %s", target.Label, target.Path)
		srv := &http.Server{Handler: target.Handler}
		servers = append(servers, srv)
		go func(srv *http.Server) {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
				errCh <- fmt.Errorf("listener %s failed: %w", target.Path, err)
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

func serveHTTPListener(ctx context.Context, srv *http.Server, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
			return err
		}
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}
}
