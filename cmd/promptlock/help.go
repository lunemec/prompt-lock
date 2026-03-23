package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "--" {
			return false
		}
		switch strings.TrimSpace(arg) {
		case "-h", "--help":
			return true
		}
	}
	return false
}

func printFlagHelp(out io.Writer, intro string, fs *flag.FlagSet) {
	fmt.Fprint(out, intro)
	if !strings.HasSuffix(intro, "\n") {
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Flags:")
	fs.SetOutput(out)
	fs.PrintDefaults()
}

func promptlockHelpText() string {
	return strings.TrimSpace(`
PromptLock CLI
Human-approved secret access for autonomous agents.

`+"`promptlock`"+` is the client CLI. `+"`promptlockd`"+` is the host broker daemon.

Usage:
  promptlock <command> [flags]

Recommended first command: promptlock setup

Evaluator flow:
  1. promptlock setup       Generate a host-side quickstart for this workspace
  2. promptlock daemon start
                            Start the broker on the host
  3. promptlock watch       Approve or deny requests from the host/operator side
  4. promptlock auth docker-run
                            Launch a containerized agent with only agent-side PromptLock transport

Commands:
  setup       Generate a hardened local quickstart for this workspace
  daemon      Start/stop/check the local promptlockd broker process
  watch       Review, list, approve, or deny pending requests from the operator side
  exec        Request secrets, wait for approval, and run locally or via broker-exec
  auth        Bootstrap, pair, mint, or launch a containerized agent session
  mcp         Diagnose PromptLock MCP launcher/client preflight issues
  audit-verify Verify the audit log hash chain on the host
               Auto-detects the workspace quickstart audit file when available
               Use --print-file to show the resolved audit file path

Run "promptlock <command> --help" for command-specific guidance.
`) + "\n"
}

func authHelpText() string {
	return strings.TrimSpace(`
PromptLock auth commands

Usage:
  promptlock auth <bootstrap|pair|mint|login|docker-run> [flags]

Recommended container quickstart: promptlock auth docker-run

Use `+"`docker-run`"+` from the host when you want the CLI to:
  1. bootstrap on the operator socket,
  2. pair and mint on the agent socket,
  3. launch `+"`docker run`"+` with only agent-side PromptLock transport and session env wired into the container.

Subcommands:
  bootstrap   Create a short-lived bootstrap token from the operator side
  pair        Exchange a bootstrap token for a grant on the agent side
  mint        Mint an agent session token from a grant id
  login       Run bootstrap + pair + mint and print safe session metadata
  docker-run  Mint a session and launch docker run with agent-side PromptLock transport/session env wired in

Use "promptlock auth <subcommand> --help" for subcommand flags.
`) + "\n"
}

func watchHelpText() string {
	return strings.TrimSpace(`
PromptLock watch

Usage:
  promptlock watch [flags]
  promptlock watch list [flags]
  promptlock watch allow --ttl 5 <request_id>
  promptlock watch deny --reason "scope too broad" <request_id>

By default, `+"`promptlock watch`"+` auto-starts a local `+"`promptlockd`"+` daemon when needed.
Use `+"`--external`"+` to connect to an already-running daemon only.
When a workspace setup from `+"`promptlock setup`"+` exists, host-side watch commands auto-detect its config, operator token, and operator socket from the workspace root.
Use this from the host/operator side to review pending requests and make approval decisions.
TTY terminals use a keyboard-driven interactive watch UI (`+"`j`"+`/`+"`k`"+` select, `+"`y`"+` approve, `+"`n`"+` deny, `+"`s`"+` skip, `+"`q`"+` quit, `+"`up`"+`/`+"`down`"+` history).
Non-interactive or redirected sessions fall back to the plain prompt/output path.
Use `+"`promptlock audit-verify --print-file`"+` to discover the current audit log path, or `+"`promptlock audit-verify`"+` to verify it.
`) + "\n"
}

func auditVerifyHelpText() string {
	return strings.TrimSpace(`
PromptLock audit-verify

Usage:
  promptlock audit-verify [flags]

Verify the current audit JSONL hash chain on the host.
Use `+"`--print-file`"+` to show the resolved audit file path before verifying.
When run from a workspace created with `+"`promptlock setup`"+`, the audit path is auto-detected.
Use `+"`--checkpoint`"+` together with `+"`--write-checkpoint`"+` for incremental verification.
`) + "\n"
}

func execHelpText() string {
	return strings.TrimSpace(`
PromptLock exec

Usage:
  promptlock exec [flags] -- <command> [args...]

By default, this command waits for human approval.
`+"`--broker-exec`"+` runs the approved command on the broker host, not inside the agent container.
In the hardened path, use `+"`--intent`"+` with `+"`--broker-exec`"+`.
In hardened broker-exec mode, prefer direct commands such as `+"`go test ./...`"+` rather than shell wrappers.
`+"`--auto-approve`"+` is for local demo flows only and requires `+"`PROMPTLOCK_DEV_MODE=1`"+`.
`) + "\n"
}

func daemonHelpText() string {
	return strings.TrimSpace(`
PromptLock daemon

Usage:
  promptlock daemon <start|stop|status> [flags]

Examples:
  promptlock daemon start
  promptlock daemon status
  promptlock daemon status --json
  promptlock daemon stop

Notes:
  - This command manages a local `+"`promptlockd`"+` process using a PID file.
  - When no explicit config is set and the current workspace already has a `+"`promptlock setup`"+` instance, the daemon command auto-detects that config.
  - When `+"`PROMPTLOCK_CONFIG`"+` or `+"`--config`"+` points at a setup instance, the default PID and log files live next to that config so separate workspaces do not collide.
  - In local hardened dual-socket mode on non-Linux hosts, the daemon also starts an agent-only loopback bridge for desktop-Docker containers.
  - `+"`promptlock daemon status`"+` also probes the local agent API and reports bridge reachability when available.
  - If you do not set `+"`--log-file`"+`, detached daemon stdout/stderr is discarded unless a config-scoped default log file applies.
  - Use `+"`--binary`"+` when `+"`promptlockd`"+` is not discoverable in PATH.
`) + "\n"
}

func setupHelpText() string {
	return strings.TrimSpace(`
PromptLock setup

Recommended first command for a new workspace.

Usage:
  promptlock setup [flags]

Creates a per-workspace quickstart instance outside the repo with:
  - config.json
  - instance.env
  - audit/state/auth files
  - role-separated agent/operator unix sockets
  - on Unix hosts, a short runtime socket dir to avoid local path-length failures
  - on non-Linux hosts, a daemon-managed agent bridge URL for desktop-Docker containers

Running setup again reuses the existing complete instance instead of rotating secrets or rewriting files.
When you later run host-side `+"`promptlock daemon start`"+`, `+"`promptlock watch`"+`, or `+"`promptlock auth docker-run`"+` from the workspace root, the CLI auto-detects this quickstart for you.
Source `+"`instance.env`"+` only when you want to run the quickstart from another directory or inspect the generated values directly.
The generated quickstart defaults to `+"`output_security_mode=raw`"+` so the first broker-exec demo can print output.
After you verify the flow, switch back to `+"`output_security_mode=none`"+` for stronger containment.
`) + "\n"
}

func mcpHelpText() string {
	return strings.TrimSpace(`
PromptLock mcp

Usage:
  promptlock mcp doctor [flags]

Use this from the shell where you plan to launch an MCP-capable client.
The doctor checks:
  - `+"`promptlock-mcp-launch`"+` availability on PATH
  - `+"`promptlock-mcp`"+` resolution for the launcher
  - the shared Codex `+"`promptlock`"+` MCP registration when a Codex config exists
  - current PromptLock session and transport env
  - a real stdio `+"`initialize`"+` / `+"`tools/list`"+` handshake through the launcher

The `+"`execute_with_intent`"+` MCP tool accepts `+"`intent`"+`, `+"`command`"+`, optional `+"`ttl_minutes`"+`,
and optional `+"`env_path`"+` for approved `+"`.env`"+` / env-like file lookup.

Subcommands:
  doctor      Run MCP preflight checks for the current shell/client context
`) + "\n"
}

func mcpDoctorHelpText() string {
	return strings.TrimSpace(`
PromptLock mcp doctor

Usage:
  promptlock mcp doctor [--json]

This command is a preflight for the exact shell where your MCP client will run.
It diagnoses the common failure modes that cause PromptLock MCP startup failures:
  - missing `+"`promptlock-mcp-launch`"+` or `+"`promptlock-mcp`"+`
  - stale or non-portable Codex MCP command configuration
  - missing live `+"`PROMPTLOCK_SESSION_TOKEN`"+`
  - missing `+"`PROMPTLOCK_AGENT_UNIX_SOCKET`"+` or `+"`PROMPTLOCK_BROKER_URL`"+`
  - launcher startup failures before the MCP `+"`initialize`"+` handshake
`) + "\n"
}
