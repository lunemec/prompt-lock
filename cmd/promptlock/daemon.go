package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultDaemonPIDFile = "/tmp/promptlockd.pid"

type daemonFlags struct {
	PIDFile string
	Binary  string
	Config  string
	LogFile string
}

func runDaemon(args []string) {
	if len(args) == 0 || hasHelpFlag(args) || args[0] == "help" {
		fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
		fs.String("pid-file", getenv("PROMPTLOCK_DAEMON_PID_FILE", defaultDaemonPIDFile), "pid file path")
		fs.String("binary", getenv("PROMPTLOCK_DAEMON_BINARY", "promptlockd"), "promptlockd binary path or name")
		fs.String("config", getenv("PROMPTLOCK_CONFIG", ""), "optional config path exported as PROMPTLOCK_CONFIG")
		fs.String("log-file", getenv("PROMPTLOCK_DAEMON_LOG_FILE", ""), "optional daemon log file path")
		printFlagHelp(os.Stdout, daemonHelpText(), fs)
		return
	}

	sub := strings.TrimSpace(args[0])
	fs := flag.NewFlagSet("daemon "+sub, flag.ExitOnError)
	pidFile := fs.String("pid-file", getenv("PROMPTLOCK_DAEMON_PID_FILE", defaultDaemonPIDFile), "pid file path")
	binary := fs.String("binary", getenv("PROMPTLOCK_DAEMON_BINARY", "promptlockd"), "promptlockd binary path or name")
	config := fs.String("config", getenv("PROMPTLOCK_CONFIG", ""), "optional config path exported as PROMPTLOCK_CONFIG")
	logFile := fs.String("log-file", getenv("PROMPTLOCK_DAEMON_LOG_FILE", ""), "optional daemon log file path")
	_ = fs.Parse(args[1:])

	flags := daemonFlags{
		PIDFile: strings.TrimSpace(*pidFile),
		Binary:  strings.TrimSpace(*binary),
		Config:  strings.TrimSpace(*config),
		LogFile: strings.TrimSpace(*logFile),
	}

	switch sub {
	case "start":
		if err := daemonStart(flags); err != nil {
			fatal(err)
		}
	case "stop":
		if err := daemonStop(flags); err != nil {
			fatal(err)
		}
	case "status":
		if err := daemonStatus(flags); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unknown daemon command: %s", sub))
	}
}

func daemonStart(flags daemonFlags) error {
	if err := ensureParentDir(flags.PIDFile); err != nil {
		return err
	}
	if pid, ok := daemonReadLivePID(flags.PIDFile); ok {
		fmt.Printf("promptlockd already running (pid=%d)\n", pid)
		return nil
	}
	if err := os.Remove(flags.PIDFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale pid file: %w", err)
	}

	binPath, err := exec.LookPath(flags.Binary)
	if err != nil {
		return fmt.Errorf("resolve promptlockd binary %q: %w", flags.Binary, err)
	}
	cmd := exec.Command(binPath)
	cmd.Env = daemonEnv(flags.Config)
	if flags.LogFile != "" {
		if err := ensureParentDir(flags.LogFile); err != nil {
			return err
		}
		logFH, err := os.OpenFile(flags.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open daemon log file: %w", err)
		}
		defer logFH.Close()
		cmd.Stdout = logFH
		cmd.Stderr = logFH
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start promptlockd: %w", err)
	}
	if err := os.WriteFile(flags.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("write pid file: %w", err)
	}
	fmt.Printf("started promptlockd (pid=%d)\n", cmd.Process.Pid)
	return nil
}

func daemonStop(flags daemonFlags) error {
	pid, err := readPIDFile(flags.PIDFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("promptlockd is not running (no pid file)")
			return nil
		}
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			_ = os.Remove(flags.PIDFile)
			fmt.Printf("promptlockd already exited (pid=%d)\n", pid)
			return nil
		}
		return fmt.Errorf("signal promptlockd %d: %w", pid, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			_ = os.Remove(flags.PIDFile)
			fmt.Printf("stopped promptlockd (pid=%d)\n", pid)
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("promptlockd (pid=%d) did not stop within timeout", pid)
}

func daemonStatus(flags daemonFlags) error {
	pid, err := readPIDFile(flags.PIDFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("promptlockd status: stopped")
			return nil
		}
		return err
	}
	if processAlive(pid) {
		fmt.Printf("promptlockd status: running (pid=%d)\n", pid)
		return nil
	}
	fmt.Printf("promptlockd status: stale pid file (pid=%d not running)\n", pid)
	return nil
}

func daemonReadLivePID(pidFile string) (int, bool) {
	pid, err := readPIDFile(pidFile)
	if err != nil {
		return 0, false
	}
	return pid, processAlive(pid)
}

func readPIDFile(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return 0, fmt.Errorf("pid file %s is empty", path)
	}
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid in %s", path)
	}
	return pid, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func daemonEnv(configPath string) []string {
	env := os.Environ()
	if strings.TrimSpace(configPath) == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	prefix := "PROMPTLOCK_CONFIG="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, prefix+configPath)
	return out
}

func ensureParentDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}
	parent := filepath.Dir(path)
	if strings.TrimSpace(parent) == "" || parent == "." {
		return nil
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	return nil
}
