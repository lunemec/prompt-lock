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
                            Launch a containerized agent with only the agent socket mounted

Commands:
  setup       Generate a hardened local quickstart for this workspace
  daemon      Start/stop/check the local promptlockd broker process
  watch       Review, list, approve, or deny pending requests from the operator side
  exec        Request secrets, wait for approval, and run locally or via broker-exec
  auth        Bootstrap, pair, mint, or launch a containerized agent session
  audit-verify Verify the audit log hash chain on the host

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
  3. launch `+"`docker run`"+` with only the agent socket and session env wired into the container.

Subcommands:
  bootstrap   Create a short-lived bootstrap token from the operator side
  pair        Exchange a bootstrap token for a grant on the agent side
  mint        Mint an agent session token from a grant id
  login       Run bootstrap + pair + mint and print safe session metadata
  docker-run  Mint a session and launch docker run with the agent socket/session env wired in

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
Defaults to the operator unix socket when a hardened local setup is active.
Use this from the host/operator side to review pending requests and make approval decisions.
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
  promptlock daemon stop

Notes:
  - This command manages a local `+"`promptlockd`"+` process using a PID file.
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

Running setup again reuses the existing complete instance instead of rotating secrets or rewriting files.
The generated quickstart defaults to `+"`output_security_mode=raw`"+` so the first broker-exec demo can print output.
After you verify the flow, switch back to `+"`output_security_mode=none`"+` for stronger containment.
`) + "\n"
}
