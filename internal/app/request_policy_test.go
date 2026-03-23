package app

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultRequestPolicy(t *testing.T) {
	p := DefaultRequestPolicy()
	if p.IdenticalRequestCooldown != 60*time.Second {
		t.Fatalf("expected default cooldown 60s, got %s", p.IdenticalRequestCooldown)
	}
	if p.MaxPendingPerAgent != 2 {
		t.Fatalf("expected default max pending per agent 2, got %d", p.MaxPendingPerAgent)
	}
	if !p.EnableActiveLeaseReuse {
		t.Fatalf("expected active lease reuse to default enabled")
	}
}

func TestRequestPolicyNormalize(t *testing.T) {
	p := RequestPolicy{
		IdenticalRequestCooldown: 0,
		MaxPendingPerAgent:       0,
	}
	normalized := p.Normalize()
	if normalized.IdenticalRequestCooldown != 60*time.Second {
		t.Fatalf("expected normalized cooldown 60s, got %s", normalized.IdenticalRequestCooldown)
	}
	if normalized.MaxPendingPerAgent != 2 {
		t.Fatalf("expected normalized max pending per agent 2, got %d", normalized.MaxPendingPerAgent)
	}
}

func TestEquivalenceKeyStableAcrossSecretOrdering(t *testing.T) {
	inputA := RequestPolicyInput{
		AgentID:            "agent-1",
		Secrets:            []string{"npm_token", "github_token"},
		CommandFingerprint: "cmd-fp",
		WorkdirFingerprint: "wd-fp",
	}
	inputB := RequestPolicyInput{
		AgentID:            "agent-1",
		Secrets:            []string{" github_token ", "npm_token", "github_token"},
		CommandFingerprint: "cmd-fp",
		WorkdirFingerprint: "wd-fp",
	}
	keyA := inputA.EquivalenceKey()
	keyB := inputB.EquivalenceKey()
	if keyA == "" || keyB == "" {
		t.Fatalf("expected non-empty equivalence keys")
	}
	if keyA != keyB {
		t.Fatalf("expected equivalent inputs to produce same key, got %q vs %q", keyA, keyB)
	}
}

func TestEquivalenceKeyChangesWhenFingerprintsDiffer(t *testing.T) {
	input := RequestPolicyInput{
		AgentID:            "agent-1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "cmd-fp",
		WorkdirFingerprint: "wd-fp",
	}
	changedCommand := RequestPolicyInput{
		AgentID:            "agent-1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "cmd-fp-2",
		WorkdirFingerprint: "wd-fp",
	}
	changedWorkdir := RequestPolicyInput{
		AgentID:            "agent-1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "cmd-fp",
		WorkdirFingerprint: "wd-fp-2",
	}

	base := input.EquivalenceKey()
	if base == changedCommand.EquivalenceKey() {
		t.Fatalf("expected command fingerprint changes to alter equivalence key")
	}
	if base == changedWorkdir.EquivalenceKey() {
		t.Fatalf("expected workdir fingerprint changes to alter equivalence key")
	}
}

func TestSummarizeCommandArgsSanitizesAndTruncates(t *testing.T) {
	got := SummarizeCommandArgs([]string{
		"git",
		"commit",
		"--message",
		"hello\tworld\nline2",
		strings.Repeat("x", 96),
		"ignored",
		"extra",
	})
	if strings.ContainsAny(got, "\n\r\t") {
		t.Fatalf("expected command summary to be sanitized, got %q", got)
	}
	if !strings.Contains(got, "... (+1 args)") {
		t.Fatalf("expected truncated summary to note omitted args, got %q", got)
	}
	if len([]rune(got)) > maxRequestCommandSummaryRunes {
		t.Fatalf("expected summary to be capped at %d runes, got %d", maxRequestCommandSummaryRunes, len([]rune(got)))
	}
}

func TestSummarizeWorkdirPathNormalizesAndTruncates(t *testing.T) {
	got := SummarizeWorkdirPath("  ./workspace//nested/../repo  ")
	if got != "workspace/repo" {
		t.Fatalf("expected cleaned workdir summary, got %q", got)
	}

	long := SummarizeWorkdirPath(strings.Repeat("a", maxRequestWorkdirSummaryRunes+20))
	if len([]rune(long)) > maxRequestWorkdirSummaryRunes {
		t.Fatalf("expected workdir summary cap at %d runes, got %d", maxRequestWorkdirSummaryRunes, len([]rune(long)))
	}
}
