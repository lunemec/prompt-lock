package main

import (
	"context"
	"encoding/json"
	"net/http"
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
	if !s.requireDurabilityReady(w) {
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
		if auditErr := s.auditCritical(ports.AuditEvent{Event: "network_egress_blocked", Timestamp: s.now(), ActorType: at, ActorID: aid, Metadata: map[string]string{"reason": err.Error(), "command": strings.Join(req.Command, " ")}}); auditErr != nil {
			writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
			return
		}
		writeMappedError(w, ErrForbidden, withPolicyHint(err.Error()))
		return
	}
	if _, err := s.ensureEnvPathSecretStore(); err != nil {
		writeMappedError(w, ErrServiceUnavailable, err.Error())
		return
	}

	_, actorID := actorFromRequest(r)
	if !s.authEnabled {
		actorID = ""
	}
	resolved, err := s.svc.ResolveExecutionSecretsByAgent(actorID, req.LeaseToken, req.Secrets, req.CommandFingerprint, req.WorkdirFingerprint)
	if err != nil {
		kind, msg := stateStoreAccessError(err)
		writeMappedError(w, kind, msg)
		return
	}

	timeoutSec := s.controlPolicy().ClampTimeout(req.TimeoutSec)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	resolvedCommand, err := s.controlPolicy().ResolveExecuteCommand(req.Command)
	if err != nil {
		writeMappedError(w, ErrForbidden, withPolicyHint(err.Error()))
		return
	}
	at, aid := actorFromRequest(r)
	if err := s.auditCritical(ports.AuditEvent{
		Event:      "execute_with_secret_started",
		Timestamp:  s.now(),
		ActorType:  at,
		ActorID:    aid,
		LeaseToken: req.LeaseToken,
		Metadata: map[string]string{
			"command":     strings.Join(req.Command, " "),
			"timeout_sec": itoa(uint64(timeoutSec)),
		},
	}); err != nil {
		writeMappedError(w, ErrServiceUnavailable, durabilityUnavailableMessage)
		return
	}
	cmd := exec.CommandContext(ctx, resolvedCommand.Path, resolvedCommand.Args...)
	if len(req.Command) > 0 {
		cmd.Args[0] = req.Command[0]
	}
	cmd.Env = buildBrokerExecutionEnv(resolved, resolvedCommand.SearchPath)
	captureLimit := effectiveOutputCaptureLimit(s.execPolicy.OutputSecurityMode, s.execPolicy.MaxOutputBytes)
	out, code, err := runCommandWithBoundedOutput(cmd, captureLimit)
	if err != nil {
		writeMappedError(w, ErrInternal, err.Error())
		return
	}
	outStr := s.controlPolicy().ApplyOutputSecurity(out)
	if max := s.execPolicy.MaxOutputBytes; max > 0 && len(outStr) > max {
		outStr = outStr[:max]
	}
	auditWarning := ""
	if err := s.auditCritical(ports.AuditEvent{
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
	}); err != nil {
		auditWarning = durabilityUnavailableMessage
	}
	resp := map[string]any{"exit_code": code, "stdout_stderr": outStr}
	if auditWarning != "" {
		resp["audit_warning"] = auditWarning
	}
	writeJSON(w, resp)
}
