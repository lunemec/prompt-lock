package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultDaemonPIDFile     = "/tmp/promptlockd.pid"
	defaultDaemonPIDFileName = "promptlockd.pid"
	defaultDaemonLogFileName = "promptlockd.log"
	defaultDaemonBinaryName  = "promptlockd-autostart"
)

var (
	daemonLookPath           = exec.LookPath
	daemonBuildSourceBinary  = buildDaemonBinaryFromSource
	daemonReadProcessState   = readDaemonProcessState
	daemonSourceBuildAllowed = daemonSourceBuildAvailable
)

type daemonFlags struct {
	PIDFile    string
	Binary     string
	Config     string
	LogFile    string
	JSONOutput bool
}

func runDaemon(args []string) {
	if len(args) == 0 || hasHelpFlag(args) || args[0] == "help" {
		fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
		fs.String("pid-file", getenv("PROMPTLOCK_DAEMON_PID_FILE", ""), "pid file path (defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
		fs.String("binary", getenv("PROMPTLOCK_DAEMON_BINARY", "promptlockd"), "promptlockd binary path or name")
		fs.String("config", getenv("PROMPTLOCK_CONFIG", ""), "optional config path exported as PROMPTLOCK_CONFIG")
		fs.String("log-file", getenv("PROMPTLOCK_DAEMON_LOG_FILE", ""), "optional daemon log file path (defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
		fs.Bool("json", false, "print structured status JSON (status subcommand)")
		printFlagHelp(os.Stdout, daemonHelpText(), fs)
		return
	}

	sub := strings.TrimSpace(args[0])
	fs := flag.NewFlagSet("daemon "+sub, flag.ExitOnError)
	pidFile := fs.String("pid-file", getenv("PROMPTLOCK_DAEMON_PID_FILE", ""), "pid file path (defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
	binary := fs.String("binary", getenv("PROMPTLOCK_DAEMON_BINARY", "promptlockd"), "promptlockd binary path or name")
	config := fs.String("config", getenv("PROMPTLOCK_CONFIG", ""), "optional config path exported as PROMPTLOCK_CONFIG")
	logFile := fs.String("log-file", getenv("PROMPTLOCK_DAEMON_LOG_FILE", ""), "optional daemon log file path (defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
	jsonOutput := fs.Bool("json", false, "print structured status JSON (status subcommand)")
	_ = fs.Parse(args[1:])

	flags := newDaemonFlags(*pidFile, *binary, *config, *logFile, *jsonOutput)

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

	cmd, launchDesc, err := buildDaemonCommand(flags)
	if err != nil {
		return err
	}
	cmd.Env = daemonEnv(flags.Config)
	cmd.Stdin = nil
	cmd.SysProcAttr = daemonSysProcAttr()
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
		nullFH, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("open null daemon output: %w", err)
		}
		defer nullFH.Close()
		cmd.Stdout = nullFH
		cmd.Stderr = nullFH
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start promptlockd: %w", err)
	}
	if err := os.WriteFile(flags.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("write pid file: %w", err)
	}
	fmt.Printf("started promptlockd (pid=%d) via %s\n", cmd.Process.Pid, launchDesc)
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
	if !processAlive(pid) {
		_ = os.Remove(flags.PIDFile)
		fmt.Printf("promptlockd already exited (pid=%d)\n", pid)
		return nil
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
	if err != nil {
		return false
	}
	state, err := daemonReadProcessState(pid)
	if err != nil {
		return true
	}
	state = strings.TrimSpace(strings.ToUpper(state))
	if strings.HasPrefix(state, "Z") {
		return false
	}
	return true
}

func buildDaemonCommand(flags daemonFlags) (*exec.Cmd, string, error) {
	if path, err := daemonLookPath(flags.Binary); err == nil {
		return exec.Command(path), path, nil
	}
	if strings.TrimSpace(flags.Binary) == "promptlockd" && daemonSourceBuildAllowed() {
		if builtPath, buildErr := daemonBuildSourceBinary(flags.Config); buildErr == nil {
			return exec.Command(builtPath), fmt.Sprintf("%s (built from source)", builtPath), nil
		}
	}
	return nil, "", fmt.Errorf("resolve promptlockd binary %q: executable not found", flags.Binary)
}

func daemonSourceBuildAvailable() bool {
	return fileExists("go.mod") && fileExists(filepath.Join("cmd", "promptlockd", "main.go"))
}

func buildDaemonBinaryFromSource(configPath string) (string, error) {
	goPath, err := daemonLookPath("go")
	if err != nil {
		return "", fmt.Errorf("resolve go tool for promptlockd source build: %w", err)
	}
	targetPath, err := daemonBuiltBinaryPath(configPath)
	if err != nil {
		return "", err
	}
	if err := ensureParentDir(targetPath); err != nil {
		return "", err
	}

	tempPath := targetPath + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	buildCmd := exec.Command(goPath, "build", "-o", tempPath, "./cmd/promptlockd")
	buildCmd.Env = os.Environ()
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tempPath)
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", fmt.Errorf("build promptlockd from source: %w", err)
		}
		return "", fmt.Errorf("build promptlockd from source: %w: %s", err, trimmed)
	}
	_ = os.Remove(targetPath)
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("activate built promptlockd binary: %w", err)
	}
	return targetPath, nil
}

func daemonBuiltBinaryPath(configPath string) (string, error) {
	name := defaultDaemonBinaryName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if trimmedConfig := strings.TrimSpace(resolvedWorkspaceSetupConfigPath(configPath)); trimmedConfig != "" {
		return filepath.Join(filepath.Dir(trimmedConfig), name), nil
	}

	cwd, err := setupGetwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory for daemon binary path: %w", err)
	}
	sum := sha256.Sum256([]byte(cwd))
	suffix := hex.EncodeToString(sum[:8])
	return filepath.Join(os.TempDir(), name+"-"+suffix), nil
}

func readDaemonProcessState(pid int) (string, error) {
	if runtime.GOOS == "windows" {
		return "", nil
	}
	psPath, err := daemonLookPath("ps")
	if err != nil {
		return "", err
	}
	out, err := exec.Command(psPath, "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func newDaemonFlags(pidFile, binary, configPath, logFile string, jsonOutput bool) daemonFlags {
	trimmedConfig := resolvedWorkspaceSetupConfigPath(strings.TrimSpace(configPath))
	return daemonFlags{
		PIDFile:    resolveDaemonPIDFile(pidFile, trimmedConfig),
		Binary:     strings.TrimSpace(binary),
		Config:     trimmedConfig,
		LogFile:    resolveDaemonLogFile(logFile, trimmedConfig),
		JSONOutput: jsonOutput,
	}
}

func resolveDaemonPIDFile(pidFile, configPath string) string {
	if trimmed := strings.TrimSpace(pidFile); trimmed != "" {
		return trimmed
	}
	if trimmedConfig := strings.TrimSpace(configPath); trimmedConfig != "" {
		return filepath.Join(filepath.Dir(trimmedConfig), defaultDaemonPIDFileName)
	}
	return defaultDaemonPIDFile
}

func resolveDaemonLogFile(logFile, configPath string) string {
	if trimmed := strings.TrimSpace(logFile); trimmed != "" {
		return trimmed
	}
	if trimmedConfig := strings.TrimSpace(configPath); trimmedConfig != "" {
		return filepath.Join(filepath.Dir(trimmedConfig), defaultDaemonLogFileName)
	}
	return ""
}

func daemonEnv(configPath string) []string {
	resolvedConfigPath := resolvedWorkspaceSetupConfigPath(configPath)
	env := os.Environ()
	if strings.TrimSpace(resolvedConfigPath) == "" {
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
	out = append(out, prefix+resolvedConfigPath)
	return mergeMissingEnv(out, loadWorkspaceSetupEnvExports(resolvedConfigPath))
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
