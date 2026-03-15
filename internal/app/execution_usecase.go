package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/core/ports"
)

const defaultOutputCaptureBytes = 64 * 1024

type CommandRunRequest struct {
	Command      ResolvedCommand
	Argv0        string
	Env          []string
	CaptureLimit int
}

type CommandRunResult struct {
	Output   string
	ExitCode int
}

type CommandRunner interface {
	Run(ctx context.Context, req CommandRunRequest) (CommandRunResult, error)
}

type CommandExecutionOutput struct {
	ExitCode     int
	StdoutStderr string
	AuditWarning string
}

type ExecuteWithLeaseInput struct {
	ActorType           string
	ActorID             string
	LeaseToken          string
	Intent              string
	Command             []string
	Secrets             []string
	CommandFingerprint  string
	WorkdirFingerprint  string
	RequestedTimeoutSec int
}

type ExecuteWithLeaseUseCase struct {
	Service             *Service
	Policy              ControlPlanePolicy
	SecurityProfile     string
	Runner              CommandRunner
	MaxOutputBytes      int
	OutputMode          string
	AmbientEnv          []string
	AuditFailureWarning string
}

func (u ExecuteWithLeaseUseCase) Execute(ctx context.Context, input ExecuteWithLeaseInput) (CommandExecutionOutput, error) {
	if u.Service == nil || u.Policy == nil || u.Runner == nil {
		return CommandExecutionOutput{}, errors.New("execute use case is not fully configured")
	}
	if len(input.Secrets) == 0 {
		return CommandExecutionOutput{}, errors.New("secrets are required")
	}
	if err := u.Policy.ValidateExecuteRequest(u.SecurityProfile, ExecuteRequest{Intent: input.Intent, Command: input.Command}); err != nil {
		return CommandExecutionOutput{}, err
	}

	approvedIntent, err := u.Service.ApprovedLeaseIntentByAgent(input.LeaseToken, strings.TrimSpace(input.ActorID))
	if err != nil {
		return CommandExecutionOutput{}, err
	}
	if strings.TrimSpace(approvedIntent) != "" && strings.TrimSpace(input.Intent) != strings.TrimSpace(approvedIntent) {
		return CommandExecutionOutput{}, fmt.Errorf("execute intent does not match approved request intent")
	}
	if err := u.Policy.ValidateNetworkEgress(input.Command, approvedIntent); err != nil {
		if auditErr := u.Service.auditCritical(ports.AuditEvent{
			Event:     "network_egress_blocked",
			Timestamp: u.Service.now(),
			ActorType: strings.TrimSpace(input.ActorType),
			ActorID:   strings.TrimSpace(input.ActorID),
			Metadata: map[string]string{
				"reason":  err.Error(),
				"command": strings.Join(input.Command, " "),
			},
		}); auditErr != nil {
			return CommandExecutionOutput{}, auditErr
		}
		return CommandExecutionOutput{}, err
	}

	resolvedCommand, err := u.Policy.ResolveExecuteCommand(input.Command)
	if err != nil {
		return CommandExecutionOutput{}, err
	}

	resolvedSecrets, err := u.Service.ResolveExecutionSecretsByAgent(strings.TrimSpace(input.ActorID), input.LeaseToken, input.Secrets, input.CommandFingerprint, input.WorkdirFingerprint)
	if err != nil {
		return CommandExecutionOutput{}, err
	}

	timeoutSec := u.Policy.ClampTimeout(input.RequestedTimeoutSec)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()
	if err := u.Service.auditCritical(ports.AuditEvent{
		Event:      "execute_with_secret_started",
		Timestamp:  u.Service.now(),
		ActorType:  strings.TrimSpace(input.ActorType),
		ActorID:    strings.TrimSpace(input.ActorID),
		LeaseToken: input.LeaseToken,
		Metadata: map[string]string{
			"command":     strings.Join(input.Command, " "),
			"timeout_sec": strconv.FormatUint(uint64(timeoutSec), 10),
		},
	}); err != nil {
		return CommandExecutionOutput{}, err
	}

	runResult, err := u.Runner.Run(runCtx, CommandRunRequest{
		Command:      resolvedCommand,
		Argv0:        firstCommandArg(input.Command),
		Env:          BuildExecutionEnvironmentWithPathOverride(u.ambientEnv(), resolvedSecrets, resolvedCommand.SearchPath),
		CaptureLimit: effectiveOutputCaptureLimit(u.OutputMode, u.MaxOutputBytes),
	})
	if err != nil {
		return CommandExecutionOutput{}, fmt.Errorf("%w: %v", ErrCommandExecutionFailed, err)
	}

	output := u.Policy.ApplyOutputSecurity(runResult.Output)
	if u.MaxOutputBytes > 0 && len(output) > u.MaxOutputBytes {
		output = output[:u.MaxOutputBytes]
	}
	auditWarning := ""
	if err := u.Service.auditCritical(ports.AuditEvent{
		Event:      "execute_with_secret",
		Timestamp:  u.Service.now(),
		ActorType:  strings.TrimSpace(input.ActorType),
		ActorID:    strings.TrimSpace(input.ActorID),
		LeaseToken: input.LeaseToken,
		Metadata: map[string]string{
			"command":     strings.Join(input.Command, " "),
			"exit_code":   strconv.FormatUint(uint64(runResult.ExitCode), 10),
			"timeout_sec": strconv.FormatUint(uint64(timeoutSec), 10),
		},
	}); err != nil {
		auditWarning = u.auditFailureWarning()
	}

	return CommandExecutionOutput{
		ExitCode:     runResult.ExitCode,
		StdoutStderr: output,
		AuditWarning: auditWarning,
	}, nil
}

type HostDockerExecuteInput struct {
	ActorType string
	ActorID   string
	Command   []string
}

type HostDockerExecuteUseCase struct {
	Service             *Service
	Policy              ControlPlanePolicy
	Runner              CommandRunner
	MaxOutputBytes      int
	OutputMode          string
	AmbientEnv          []string
	TimeoutSec          int
	AuditFailureWarning string
}

func (u HostDockerExecuteUseCase) Execute(ctx context.Context, input HostDockerExecuteInput) (CommandExecutionOutput, error) {
	if u.Service == nil || u.Policy == nil || u.Runner == nil {
		return CommandExecutionOutput{}, errors.New("host docker execute use case is not fully configured")
	}
	if err := u.Policy.ValidateHostDockerCommand(input.Command); err != nil {
		return CommandExecutionOutput{}, err
	}
	timeoutSec := u.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	resolvedCommand, err := u.Policy.ResolveHostDockerCommand(input.Command)
	if err != nil {
		return CommandExecutionOutput{}, err
	}
	if err := u.Service.auditCritical(ports.AuditEvent{
		Event:     "host_docker_execute_started",
		Timestamp: u.Service.now(),
		ActorType: strings.TrimSpace(input.ActorType),
		ActorID:   strings.TrimSpace(input.ActorID),
		Metadata: map[string]string{
			"command":     strings.Join(input.Command, " "),
			"timeout_sec": strconv.FormatUint(uint64(timeoutSec), 10),
		},
	}); err != nil {
		return CommandExecutionOutput{}, err
	}

	runResult, err := u.Runner.Run(runCtx, CommandRunRequest{
		Command:      resolvedCommand,
		Argv0:        firstCommandArg(input.Command),
		Env:          BuildExecutionEnvironmentWithPathOverride(u.ambientEnv(), nil, resolvedCommand.SearchPath),
		CaptureLimit: effectiveOutputCaptureLimit(u.outputMode("redacted"), u.MaxOutputBytes),
	})
	if err != nil {
		return CommandExecutionOutput{}, fmt.Errorf("%w: %v", ErrCommandExecutionFailed, err)
	}

	output := applyOutputMode(u.outputMode("redacted"), runResult.Output)
	if u.MaxOutputBytes > 0 && len(output) > u.MaxOutputBytes {
		output = output[:u.MaxOutputBytes]
	}
	auditWarning := ""
	if err := u.Service.auditCritical(ports.AuditEvent{
		Event:     "host_docker_execute",
		Timestamp: u.Service.now(),
		ActorType: strings.TrimSpace(input.ActorType),
		ActorID:   strings.TrimSpace(input.ActorID),
		Metadata: map[string]string{
			"command":   strings.Join(input.Command, " "),
			"exit_code": strconv.FormatUint(uint64(runResult.ExitCode), 10),
		},
	}); err != nil {
		auditWarning = u.auditFailureWarning()
	}

	return CommandExecutionOutput{
		ExitCode:     runResult.ExitCode,
		StdoutStderr: output,
		AuditWarning: auditWarning,
	}, nil
}

func (u ExecuteWithLeaseUseCase) ambientEnv() []string {
	return append([]string(nil), u.AmbientEnv...)
}

func (u HostDockerExecuteUseCase) ambientEnv() []string {
	return append([]string(nil), u.AmbientEnv...)
}

func (u ExecuteWithLeaseUseCase) auditFailureWarning() string {
	if strings.TrimSpace(u.AuditFailureWarning) != "" {
		return strings.TrimSpace(u.AuditFailureWarning)
	}
	return "durability persistence unavailable; broker closed for safety"
}

func (u HostDockerExecuteUseCase) auditFailureWarning() string {
	if strings.TrimSpace(u.AuditFailureWarning) != "" {
		return strings.TrimSpace(u.AuditFailureWarning)
	}
	return "durability persistence unavailable; broker closed for safety"
}

func (u HostDockerExecuteUseCase) outputMode(fallback string) string {
	if strings.TrimSpace(u.OutputMode) != "" {
		return strings.TrimSpace(u.OutputMode)
	}
	return fallback
}

func effectiveOutputCaptureLimit(outputMode string, maxBytes int) int {
	if strings.EqualFold(strings.TrimSpace(outputMode), "none") {
		return 0
	}
	if maxBytes > 0 {
		return maxBytes
	}
	return defaultOutputCaptureBytes
}

func applyOutputMode(mode, input string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "none":
		return ""
	case "raw":
		return input
	default:
		return redactOutput(input)
	}
}

func firstCommandArg(command []string) string {
	if len(command) == 0 {
		return ""
	}
	return command[0]
}
