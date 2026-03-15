package main

import (
	"context"
	"os/exec"

	"github.com/lunemec/promptlock/internal/app"
)

type processRunner struct{}

func (processRunner) Run(ctx context.Context, req app.CommandRunRequest) (app.CommandRunResult, error) {
	cmd := exec.CommandContext(ctx, req.Command.Path, req.Command.Args...)
	if req.Argv0 != "" && len(cmd.Args) > 0 {
		cmd.Args[0] = req.Argv0
	}
	cmd.Env = append([]string(nil), req.Env...)
	out, exitCode, err := runCommandWithBoundedOutput(cmd, req.CaptureLimit)
	if err != nil {
		return app.CommandRunResult{}, err
	}
	return app.CommandRunResult{Output: out, ExitCode: exitCode}, nil
}
