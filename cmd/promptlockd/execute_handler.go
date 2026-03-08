package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/core/ports"
)

type executeReq struct {
	LeaseToken         string   `json:"lease_token"`
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
		http.Error(w, "method not allowed", 405)
		return
	}
	var req executeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(req.Command) == 0 {
		http.Error(w, "command is required", 400)
		return
	}
	if err := s.validateExecuteCommand(req.Command); err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	if len(req.Secrets) == 0 {
		http.Error(w, "secrets are required", 400)
		return
	}
	if err := s.validateNetworkEgress(req.Command); err != nil {
		at, aid := actorFromRequest(r)
		_ = s.svc.Audit.Write(ports.AuditEvent{Event: "network_egress_blocked", Timestamp: s.now(), ActorType: at, ActorID: aid, Metadata: map[string]string{"reason": err.Error(), "command": strings.Join(req.Command, " ")}})
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	env := os.Environ()
	for _, sec := range req.Secrets {
		v, err := s.svc.AccessSecret(req.LeaseToken, sec, req.CommandFingerprint, req.WorkdirFingerprint)
		if err != nil {
			http.Error(w, err.Error(), 403)
			return
		}
		env = append(env, strings.ToUpper(sec)+"="+v)
	}

	timeoutSec := req.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = s.execPolicy.DefaultTimeoutSec
	}
	if timeoutSec > s.execPolicy.MaxTimeoutSec {
		timeoutSec = s.execPolicy.MaxTimeoutSec
	}
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
			http.Error(w, err.Error(), 500)
			return
		}
	}
	outStr := redactOutput(string(out))
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
