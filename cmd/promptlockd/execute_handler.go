package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type executeReq struct {
	LeaseToken         string   `json:"lease_token"`
	Intent             string   `json:"intent,omitempty"`
	Command            []string `json:"command"`
	Secrets            []string `json:"secrets"`
	CommandFingerprint string   `json:"command_fingerprint"`
	WorkdirFingerprint string   `json:"workdir_fingerprint"`
	TimeoutSec         int      `json:"timeout_sec"`
}

func (s *server) handleExecute(w http.ResponseWriter, r *http.Request) {
	var ok bool
	r, ok = s.requireAgentSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeMappedError(w, ErrMethodNotAllowed, "method not allowed")
		return
	}
	var req executeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMappedError(w, ErrBadRequest, err.Error())
		return
	}
	if len(req.Command) == 0 {
		writeMappedError(w, ErrBadRequest, "command is required")
		return
	}
	if err := s.controlPolicy().ValidateExecuteRequest(s.securityProfile, app.ExecuteRequest{Intent: req.Intent, Command: req.Command}); err != nil {
		writeMappedError(w, ErrForbidden, withPolicyHint(err.Error()))
		return
	}
	if len(req.Secrets) == 0 {
		writeMappedError(w, ErrBadRequest, "secrets are required")
		return
	}
	if err := s.controlPolicy().ValidateNetworkEgress(req.Command, req.Intent); err != nil {
		at, aid := actorFromRequest(r)
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "network_egress_blocked", Timestamp: s.now(), ActorType: at, ActorID: aid, Metadata: map[string]string{"reason": err.Error(), "command": strings.Join(req.Command, " ")}})
		writeMappedError(w, ErrForbidden, withPolicyHint(err.Error()))
		return
	}
	if _, err := s.ensureEnvPathSecretStore(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, err.Error())
		return
	}

	env := os.Environ()
	resolved, err := s.svc.ResolveExecutionSecrets(req.LeaseToken, req.Secrets, req.CommandFingerprint, req.WorkdirFingerprint)
	if err != nil {
		kind, msg := stateStoreAccessError(err)
		writeMappedError(w, kind, msg)
		return
	}
	for _, sec := range req.Secrets {
		name := strings.TrimSpace(sec)
		if name == "" {
			continue
		}
		v, ok := resolved[name]
		if !ok {
			writeMappedError(w, ErrForbidden, "secret resolution mismatch")
			return
		}
		env = append(env, strings.ToUpper(name)+"="+v)
	}

	timeoutSec := s.controlPolicy().ClampTimeout(req.TimeoutSec)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, req.Command[0], req.Command[1:]...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			writeMappedError(w, ErrInternal, err.Error())
			return
		}
	}
	outStr := s.controlPolicy().ApplyOutputSecurity(string(out))
	if max := s.execPolicy.MaxOutputBytes; max > 0 && len(outStr) > max {
		outStr = outStr[:max]
	}
	at, aid := actorFromRequest(r)
	_ = s.svc.Audit.Write(ports.AuditEvent{
		Event:      "execute_with_secret",
		Timestamp:  s.now(),
		ActorType:  at,
		ActorID:    aid,
		LeaseToken: req.LeaseToken,
		Metadata: map[string]string{
			"command":     strings.Join(req.Command, " "),
			"exit_code":   itoa(uint64(code)),
			"timeout_sec": itoa(uint64(timeoutSec)),
		},
	})
	writeJSON(w, map[string]any{"exit_code": code, "stdout_stderr": outStr})
}
