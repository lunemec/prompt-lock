package ptyrunner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/creack/pty"
)

type Stage struct {
	Triggers []string
	Keys     []byte
	Delay    time.Duration
}

type Options struct {
	OutputPath string
	Timeout    time.Duration
	Term       string
	Command    []string
	Stages     []Stage
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return "command exited with non-zero status"
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Run(opts Options) error {
	if strings.TrimSpace(opts.OutputPath) == "" {
		return errors.New("output path is required")
	}
	if len(opts.Command) == 0 {
		return errors.New("command is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if strings.TrimSpace(opts.Term) == "" {
		opts.Term = "xterm-256color"
	}

	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return fmt.Errorf("create transcript directory: %w", err)
	}

	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	cmd.Env = overrideEnv(os.Environ(), "TERM", opts.Term)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("start command in pty: %w", err)
	}

	var transcript bytes.Buffer
	readerErrCh := make(chan error, 1)
	chunks := make(chan []byte, 8)
	go func() {
		defer close(chunks)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				chunks <- chunk
			}
			if err != nil {
				if !errors.Is(err, io.EOF) && !isPTYEOF(err) {
					readerErrCh <- err
				}
				return
			}
		}
	}()

	timer := time.NewTimer(opts.Timeout)
	defer timer.Stop()

	stages := normalizeStages(opts.Stages)
	stageIndex := 0
	var runErr error
	var procErr error
	var readErr error
	procDone := false
	readDone := false
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	for !procDone || !readDone {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				readDone = true
				chunks = nil
				continue
			}
			transcript.Write(chunk)
			for stageIndex < len(stages) && stageMatches(transcript.String(), stages[stageIndex].Triggers) {
				stage := stages[stageIndex]
				if stage.Delay > 0 {
					time.Sleep(stage.Delay)
				}
				if _, err := ptmx.Write(stage.Keys); err != nil {
					runErr = fmt.Errorf("write staged keys: %w", err)
					goto DONE
				}
				stageIndex++
			}
		case err := <-readerErrCh:
			readErr = fmt.Errorf("read pty: %w", err)
			readDone = true
			readerErrCh = nil
			continue
		case err := <-waitCh:
			procErr = err
			procDone = true
			waitCh = nil
			continue
		case <-timer.C:
			_ = killProcessGroup(cmd.Process)
			_ = ptmx.Close()
			runErr = &ExitError{Code: 124, Err: fmt.Errorf("pty command timed out after %s: %s", opts.Timeout, strings.Join(opts.Command, " "))}
			goto DONE
		}
	}

DONE:
	_ = ptmx.Close()
	if writeErr := os.WriteFile(opts.OutputPath, transcript.Bytes(), 0o600); writeErr != nil && runErr == nil {
		runErr = fmt.Errorf("write transcript: %w", writeErr)
	}
	if runErr != nil {
		if ee := exitErrorFrom(runErr); ee != nil {
			return ee
		}
		return runErr
	}
	if procErr != nil {
		if ee := exitErrorFrom(procErr); ee != nil {
			return ee
		}
		return procErr
	}
	return readErr
}

func normalizeStages(stages []Stage) []Stage {
	out := make([]Stage, 0, len(stages))
	for _, stage := range stages {
		if len(stage.Triggers) == 0 || len(stage.Keys) == 0 {
			continue
		}
		out = append(out, stage)
	}
	return out
}

func stageMatches(transcript string, triggers []string) bool {
	for _, trigger := range triggers {
		if strings.TrimSpace(trigger) == "" {
			continue
		}
		if strings.Contains(transcript, trigger) {
			return true
		}
	}
	return false
}

func overrideEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func exitErrorFrom(err error) *ExitError {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{Code: exitErr.ExitCode(), Err: err}
	}
	return nil
}

func waitForCommand(cmd *exec.Cmd) error {
	err := cmd.Wait()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return &ExitError{Code: code, Err: err}
	}
	if code != 0 {
		return &ExitError{Code: code, Err: fmt.Errorf("command exited with status %d", code)}
	}
	return nil
}

func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}

func isPTYEOF(err error) bool {
	if err == nil {
		return false
	}
	errText := strings.ToLower(err.Error())
	return strings.Contains(errText, "input/output error") || strings.Contains(errText, "bad file descriptor")
}
