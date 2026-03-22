package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPromptlockHelpTextMentionsMCPDoctor(t *testing.T) {
	got := promptlockHelpText()
	for _, want := range []string{
		"mcp         Diagnose PromptLock MCP launcher/client preflight issues",
		"Run \"promptlock <command> --help\" for command-specific guidance.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("top-level help missing %q:\n%s", want, got)
		}
	}
}

func TestMCPHelpTextDocumentsDoctor(t *testing.T) {
	got := mcpHelpText()
	for _, want := range []string{
		"PromptLock mcp",
		"promptlock mcp doctor",
		"initialize` / `tools/list` handshake",
		"doctor      Run MCP preflight checks",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mcp help missing %q:\n%s", want, got)
		}
	}
}

func TestMCPDoctorHelpTextDocumentsFailuresItDetects(t *testing.T) {
	got := mcpDoctorHelpText()
	for _, want := range []string{
		"PromptLock mcp doctor",
		"missing `promptlock-mcp-launch` or `promptlock-mcp`",
		"stale or non-portable Codex MCP command configuration",
		"launcher startup failures before the MCP `initialize` handshake",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mcp doctor help missing %q:\n%s", want, got)
		}
	}
}

func TestReadCodexPromptlockCommandParsesPortableEntry(t *testing.T) {
	const body = `
[mcp_servers.promptlock]
command = "promptlock-mcp-launch"
`
	cmd, found, err := readCodexPromptlockCommand("config.toml", func(string) ([]byte, error) {
		return []byte(body), nil
	})
	if err != nil {
		t.Fatalf("readCodexPromptlockCommand returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected promptlock entry to be found")
	}
	if cmd != "promptlock-mcp-launch" {
		t.Fatalf("command = %q, want portable launcher command", cmd)
	}
}

func TestBuildMCPDoctorReportFailsBrokenAbsoluteCodexPath(t *testing.T) {
	env := map[string]string{
		"HOME":                     "/Users/test",
		"PROMPTLOCK_SESSION_TOKEN": "sess_live",
		"PROMPTLOCK_BROKER_URL":    "http://127.0.0.1:8765",
	}
	report := buildMCPDoctorReport(mcpDoctorDeps{
		Env: []string{
			"HOME=/Users/test",
			"PROMPTLOCK_SESSION_TOKEN=sess_live",
			"PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765",
		},
		Getenv: func(key string) string { return env[key] },
		LookPath: func(name string) (string, error) {
			switch name {
			case "promptlock-mcp-launch":
				return "/Users/test/.local/bin/promptlock-mcp-launch", nil
			case "promptlock-mcp":
				return "/Users/test/.local/bin/promptlock-mcp", nil
			case "codex":
				return "/Users/test/.local/bin/codex", nil
			default:
				return "", errors.New("missing")
			}
		},
		ReadFile: func(string) ([]byte, error) {
			return []byte("[mcp_servers.promptlock]\ncommand = \"/usr/local/bin/promptlock-mcp-launch\"\n"), nil
		},
		FileExists: func(path string) bool {
			switch path {
			case "/Users/test/.codex/config.toml", "/Users/test/.local/bin/promptlock-mcp-launch", "/Users/test/.local/bin/promptlock-mcp":
				return true
			default:
				return false
			}
		},
		ProbeLauncher: func(context.Context, string, []string) error { return nil },
	})
	if report.Ready {
		t.Fatalf("expected non-portable absolute Codex command to fail report readiness")
	}
	if !containsDoctorCheck(report.Checks, "codex_promptlock_entry", "fail") {
		t.Fatalf("expected codex_promptlock_entry failure, got %+v", report.Checks)
	}
}

func TestBuildMCPDoctorReportPassesPortableConfigAndHandshake(t *testing.T) {
	env := map[string]string{
		"HOME":                             "/Users/test",
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN": "sess_live",
		"PROMPTLOCK_WRAPPER_BROKER_URL":    "http://host.docker.internal:58879",
	}
	report := buildMCPDoctorReport(mcpDoctorDeps{
		Env: []string{
			"HOME=/Users/test",
			"PROMPTLOCK_WRAPPER_SESSION_TOKEN=sess_live",
			"PROMPTLOCK_WRAPPER_BROKER_URL=http://host.docker.internal:58879",
		},
		Getenv: func(key string) string { return env[key] },
		LookPath: func(name string) (string, error) {
			switch name {
			case "promptlock-mcp-launch":
				return "/Users/test/.local/bin/promptlock-mcp-launch", nil
			case "promptlock-mcp":
				return "/Users/test/.local/bin/promptlock-mcp", nil
			case "codex":
				return "/Users/test/.local/bin/codex", nil
			default:
				return "", errors.New("missing")
			}
		},
		ReadFile: func(string) ([]byte, error) {
			return []byte("[mcp_servers.promptlock]\ncommand = \"promptlock-mcp-launch\"\n"), nil
		},
		FileExists: func(path string) bool {
			switch path {
			case "/Users/test/.codex/config.toml", "/Users/test/.local/bin/promptlock-mcp-launch", "/Users/test/.local/bin/promptlock-mcp":
				return true
			default:
				return false
			}
		},
		ProbeLauncher: func(context.Context, string, []string) error { return nil },
	})
	if !report.Ready {
		t.Fatalf("expected portable config and handshake to produce ready report, got %+v", report)
	}
	if !containsDoctorCheck(report.Checks, "launcher_handshake", "pass") {
		t.Fatalf("expected launcher_handshake pass, got %+v", report.Checks)
	}
}

func containsDoctorCheck(checks []mcpDoctorCheck, name, status string) bool {
	for _, check := range checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}
