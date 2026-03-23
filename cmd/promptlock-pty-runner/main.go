package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/ptyrunner"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("promptlock-pty-runner", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	outputPath := fs.String("output", "", "transcript file path")
	timeout := fs.Duration("timeout", 30*time.Second, "maximum time to wait for the command")
	term := fs.String("term", "xterm-256color", "TERM value to expose to the command")
	inputs := fs.String("inputs", "y\nq\n", "newline-delimited staged keystrokes to send when watch triggers fire")
	stage1Trigger := fs.String("stage1-trigger", "Actions:", "primary trigger text that sends the first keystroke stage")
	stage1Delay := fs.Duration("stage1-delay", 200*time.Millisecond, "delay before sending the first keystroke stage")
	stage2Triggers := fs.String("stage2-triggers", "approved:,denied:", "comma-separated trigger texts that send the second keystroke stage")
	stage2Delay := fs.Duration("stage2-delay", 100*time.Millisecond, "delay before sending the second keystroke stage")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	command := fs.Args()
	if len(command) == 0 {
		fmt.Fprintln(os.Stderr, "error: command is required after --")
		return 2
	}
	if strings.TrimSpace(*outputPath) == "" {
		fmt.Fprintln(os.Stderr, "error: --output is required")
		return 2
	}
	stageKeys := splitInputs(*inputs)

	err := ptyrunner.Run(ptyrunner.Options{
		OutputPath: *outputPath,
		Timeout:    *timeout,
		Term:       *term,
		Command:    command,
		Stages: []ptyrunner.Stage{
			{Triggers: []string{*stage1Trigger}, Keys: []byte(stageKeys[0]), Delay: *stage1Delay},
			{Triggers: splitCSV(*stage2Triggers), Keys: []byte(stageKeys[1]), Delay: *stage2Delay},
		},
	})
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, err)
	if exitErr, ok := err.(*ptyrunner.ExitError); ok {
		return exitErr.Code
	}
	return 1
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func splitInputs(v string) []string {
	parts := strings.Split(v, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part+"\n")
		}
	}
	for len(out) < 2 {
		out = append(out, "")
	}
	return out[:2]
}
