package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = orig
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return buf.String()
}

func TestRunWatchListDisplaysDecisionContext(t *testing.T) {
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", "")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/requests/pending" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer op-token"; got != want {
			t.Fatalf("authorization header = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pending": []map[string]any{
				{
					"ID":                 "req-ctx-1",
					"AgentID":            "agent-1",
					"TaskID":             "task-1",
					"Intent":             "run_tests",
					"Reason":             "run tests",
					"TTLMinutes":         5,
					"Secrets":            []string{"github_token"},
					"CommandFingerprint": "cmd-fp-1",
					"WorkdirFingerprint": "wd-fp-1",
					"EnvPath":            "./.env",
					"EnvPathCanonical":   "/workspace/project/.env",
				},
			},
		})
	}))
	defer ts.Close()

	out := captureStdout(t, func() {
		runWatchList([]string{"--broker", ts.URL, "--operator-token", "op-token"})
	})

	checks := []string{
		"req-ctx-1",
		"agent=agent-1",
		"task=task-1",
		"intent=run_tests",
		"command_fp=cmd-fp-1",
		"workdir_fp=wd-fp-1",
		"env_path=./.env",
		"env_path_canonical=/workspace/project/.env",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected approve list output to contain %q, got %q", want, out)
		}
	}
}

func TestPromptWatchDecisionBlankLineReprompts(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("\ny\n"))
	var out bytes.Buffer

	got, err := promptWatchDecision(input, &out)
	if err != nil {
		t.Fatalf("promptWatchDecision returned error: %v", err)
	}
	if got != "approve" {
		t.Fatalf("decision = %q, want approve", got)
	}
	if strings.Count(out.String(), "Action? [y]es / [n]o / [s]kip / [q]uit: ") != 2 {
		t.Fatalf("expected prompt to be shown twice, got output %q", out.String())
	}
	if !strings.Contains(out.String(), "Enter y, n, s, or q.") {
		t.Fatalf("expected invalid-input guidance in output, got %q", out.String())
	}
}

func TestPromptWatchDecisionQuit(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("q\n"))
	var out bytes.Buffer

	got, err := promptWatchDecision(input, &out)
	if err != nil {
		t.Fatalf("promptWatchDecision returned error: %v", err)
	}
	if got != "quit" {
		t.Fatalf("decision = %q, want quit", got)
	}
	if strings.Count(out.String(), "Action? [y]es / [n]o / [s]kip / [q]uit: ") != 1 {
		t.Fatalf("expected single prompt, got output %q", out.String())
	}
}

func TestPromptWatchDecisionEOFLeavesPending(t *testing.T) {
	input := bufio.NewReader(strings.NewReader(""))
	var out bytes.Buffer

	_, err := promptWatchDecision(input, &out)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if strings.Count(out.String(), "Action? [y]es / [n]o / [s]kip / [q]uit: ") != 1 {
		t.Fatalf("expected single prompt before EOF, got output %q", out.String())
	}
}

func TestRunWatchLoopOnceRendersStableEmptyState(t *testing.T) {
	client := &stubWatchClient{
		snapshots: [][]pendingItem{{}},
	}
	var out bytes.Buffer

	err := runWatchLoop(client, watchOptions{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: 3 * time.Second,
		DefaultTTL:   5,
		Once:         true,
		Input:        bufio.NewReader(strings.NewReader("")),
		Output:       &out,
		Now: func() time.Time {
			return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("runWatchLoop returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Watching for pending requests...") {
		t.Fatalf("expected empty watch state, got %q", out.String())
	}
}

func TestRunWatchLoopSkipSuppressesUntilMembershipChanges(t *testing.T) {
	client := &stubWatchClient{
		snapshots: [][]pendingItem{
			{
				newPendingItem("req-1"),
				newPendingItem("req-2"),
			},
			{
				newPendingItem("req-1"),
				newPendingItem("req-2"),
			},
			{
				newPendingItem("req-1"),
				newPendingItem("req-2"),
				newPendingItem("req-3"),
			},
		},
	}
	var out bytes.Buffer

	err := runWatchLoop(client, watchOptions{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: time.Millisecond,
		DefaultTTL:   5,
		Input:        bufio.NewReader(strings.NewReader("s\ns\nq\n")),
		Output:       &out,
		Now: func() time.Time {
			return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
		},
		Pause: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("runWatchLoop returned error: %v", err)
	}
	if got := strings.Count(out.String(), "Request req-1"); got != 1 {
		t.Fatalf("expected req-1 to stay deferred across queue changes, got output %q", out.String())
	}
	if got := strings.Count(out.String(), "Request req-2"); got != 1 {
		t.Fatalf("expected req-2 to be shown once before deferral, got output %q", out.String())
	}
	if !strings.Contains(out.String(), "Request req-3") {
		t.Fatalf("expected new queue member to become visible after earlier requests were deferred, got output %q", out.String())
	}
	if client.approveCalls != 0 || client.denyCalls != 0 {
		t.Fatalf("expected no mutations, got approve=%d deny=%d", client.approveCalls, client.denyCalls)
	}
}

func TestRunWatchLoopQuitLeavesPendingUntouched(t *testing.T) {
	client := &stubWatchClient{
		snapshots: [][]pendingItem{{newPendingItem("req-quit")}},
	}
	var out bytes.Buffer

	err := runWatchLoop(client, watchOptions{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: time.Millisecond,
		DefaultTTL:   5,
		Input:        bufio.NewReader(strings.NewReader("q\n")),
		Output:       &out,
		Now: func() time.Time {
			return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("runWatchLoop returned error: %v", err)
	}
	if !strings.Contains(out.String(), "watch exited; leaving pending requests untouched") {
		t.Fatalf("expected quit message, got %q", out.String())
	}
	if client.approveCalls != 0 || client.denyCalls != 0 {
		t.Fatalf("expected no mutations, got approve=%d deny=%d", client.approveCalls, client.denyCalls)
	}
}

func TestRunWatchLoopSanitizesActionFailureOutput(t *testing.T) {
	client := &stubWatchClient{
		snapshots: [][]pendingItem{
			{newPendingItem("req-fail")},
			{newPendingItem("req-fail")},
		},
		approveErr: errors.New("broker said:\x1b[31mbad\x1b[0m"),
	}
	var out bytes.Buffer

	err := runWatchLoop(client, watchOptions{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: time.Millisecond,
		DefaultTTL:   5,
		Input:        bufio.NewReader(strings.NewReader("y\nq\n")),
		Output:       &out,
		Now: func() time.Time {
			return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
		},
		Pause: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("runWatchLoop returned error: %v", err)
	}
	rendered := out.String()
	if strings.Contains(rendered, "\x1b") {
		t.Fatalf("expected sanitized output to omit escape sequences, got %q", rendered)
	}
	if !strings.Contains(rendered, `broker said:\x1b[31mbad\x1b[0m`) {
		t.Fatalf("expected sanitized broker error in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "approve failed:") {
		t.Fatalf("expected approve failure output, got %q", rendered)
	}
}

func TestRunWatchLoopPlainFallbackReportsApprovedRequestID(t *testing.T) {
	client := &stubWatchClient{
		snapshots: [][]pendingItem{
			{newPendingItem("req-approve")},
			{},
		},
	}
	var out bytes.Buffer

	err := runWatchLoop(client, watchOptions{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: time.Millisecond,
		DefaultTTL:   5,
		Once:         true,
		Input:        bufio.NewReader(strings.NewReader("y\n")),
		Output:       &out,
		Now: func() time.Time {
			return time.Date(2026, 3, 22, 11, 0, 0, 0, time.UTC)
		},
		Pause: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("runWatchLoop returned error: %v", err)
	}
	if !strings.Contains(out.String(), "approved req-approve\n") {
		t.Fatalf("expected plain fallback approve output to include the request id, got %q", out.String())
	}
}

func TestRenderWatchScreenClearsTTY(t *testing.T) {
	var out bytes.Buffer

	renderWatchScreen(&out, watchView{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: 3 * time.Second,
		Now:          time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
		PendingCount: 1,
		Current:      ptrPendingItem(newPendingItem("req-clear")),
	}, true)

	if !strings.HasPrefix(out.String(), ansiClearScreen) {
		t.Fatalf("expected ANSI clear prefix, got %q", out.String())
	}
}

func TestRenderWatchScreenIncludesIntent(t *testing.T) {
	var out bytes.Buffer

	renderWatchScreen(&out, watchView{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: 3 * time.Second,
		Now:          time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
		PendingCount: 1,
		Current:      ptrPendingItem(newPendingItem("req-intent")),
	}, false)

	if !strings.Contains(out.String(), "Intent: run_tests") {
		t.Fatalf("expected watch screen to include intent, got %q", out.String())
	}
}

func TestRenderWatchScreenSanitizesAgentControlledFields(t *testing.T) {
	var out bytes.Buffer
	item := newPendingItem("req-\x1b[31m")
	item.AgentID = "agent\nnext"
	item.Reason = "danger\tzone"
	item.Secrets = []string{"github_token", "bad\rname"}
	item.EnvPath = "./demo\x07.env"

	renderWatchScreen(&out, watchView{
		BrokerTarget: "http://127.0.0.1:8765",
		PollInterval: 3 * time.Second,
		Now:          time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
		PendingCount: 1,
		Current:      &item,
	}, false)

	rendered := out.String()
	for _, bad := range []string{"\x1b", "\x07", "\r", "\nnext"} {
		if strings.Contains(rendered, bad) {
			t.Fatalf("expected plain watch render to sanitize %q, got %q", bad, rendered)
		}
	}
	for _, want := range []string{`req-\x1b[31m`, `agent\nnext`, `danger\tzone`, `bad\rname`, `./demo\x07.env`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected sanitized plain render to contain %q, got %q", want, rendered)
		}
	}
}

type stubWatchClient struct {
	snapshots    [][]pendingItem
	listCalls    int
	approveCalls int
	denyCalls    int
	listErr      error
	approveErr   error
	denyErr      error
}

func (c *stubWatchClient) ListPending() ([]pendingItem, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	if len(c.snapshots) == 0 {
		return nil, nil
	}
	idx := c.listCalls
	if idx >= len(c.snapshots) {
		idx = len(c.snapshots) - 1
	}
	c.listCalls++
	items := make([]pendingItem, len(c.snapshots[idx]))
	copy(items, c.snapshots[idx])
	return items, nil
}

func (c *stubWatchClient) Approve(string, int) error {
	c.approveCalls++
	return c.approveErr
}

func (c *stubWatchClient) Deny(string, string) error {
	c.denyCalls++
	return c.denyErr
}

func newPendingItem(id string) pendingItem {
	return pendingItem{
		ID:                 id,
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Intent:             "run_tests",
		Reason:             "run tests",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandSummary:     "git status --short",
		WorkdirSummary:     "/workspace",
		CommandFingerprint: "cmd-fp-1",
		WorkdirFingerprint: "wd-fp-1",
	}
}

func TestShouldWatchAutostartDaemon(t *testing.T) {
	t.Setenv("PROMPTLOCK_BROKER_URL", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	base := brokerFlags{Broker: ptrString(""), BrokerUnix: ptrString("")}
	if !shouldWatchAutostartDaemon(false, base) {
		t.Fatalf("expected watch to auto-start daemon by default")
	}
	if shouldWatchAutostartDaemon(true, base) {
		t.Fatalf("expected --external mode to disable daemon auto-start")
	}
	withBroker := brokerFlags{Broker: ptrString("http://example"), BrokerUnix: ptrString("")}
	if shouldWatchAutostartDaemon(false, withBroker) {
		t.Fatalf("expected explicit --broker to disable daemon auto-start")
	}
	withSocket := brokerFlags{Broker: ptrString(""), BrokerUnix: ptrString("/tmp/p.sock")}
	if shouldWatchAutostartDaemon(false, withSocket) {
		t.Fatalf("expected explicit --broker-unix-socket to disable daemon auto-start")
	}
	t.Setenv("PROMPTLOCK_BROKER_URL", "http://example")
	if shouldWatchAutostartDaemon(false, base) {
		t.Fatalf("expected PROMPTLOCK_BROKER_URL to disable daemon auto-start")
	}
}

func TestValidateWatchEnvPathExpectationErrorsWhenBrokerEnvPathDisabled(t *testing.T) {
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", "/tmp/demo-root")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta/capabilities" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth_enabled":                  true,
			"allow_plaintext_secret_return": false,
			"env_path_enabled":              false,
		})
	}))
	defer ts.Close()

	err := validateWatchEnvPathExpectation(ts.URL, "")
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "env_path disabled") {
		t.Fatalf("expected env_path disabled message, got %v", err)
	}
}

func TestValidateWatchEnvPathExpectationAllowsWhenBrokerEnvPathEnabled(t *testing.T) {
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", "/tmp/demo-root")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta/capabilities" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth_enabled":                  true,
			"allow_plaintext_secret_return": false,
			"env_path_enabled":              true,
		})
	}))
	defer ts.Close()

	if err := validateWatchEnvPathExpectation(ts.URL, ""); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateWatchEnvPathExpectationSkipsWhenUnset(t *testing.T) {
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", "")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s", r.URL.Path)
	}))
	defer ts.Close()

	if err := validateWatchEnvPathExpectation(ts.URL, ""); err != nil {
		t.Fatalf("expected no error when env root is unset, got %v", err)
	}
}

func TestValidateWatchEnvPathExpectationErrorsWhenCapabilityMissing(t *testing.T) {
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", "/tmp/demo-root")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta/capabilities" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth_enabled":                  true,
			"allow_plaintext_secret_return": false,
		})
	}))
	defer ts.Close()

	err := validateWatchEnvPathExpectation(ts.URL, "")
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "does not advertise env_path_enabled") {
		t.Fatalf("expected missing capability message, got %v", err)
	}
}

func ptrString(v string) *string {
	return &v
}

func ptrPendingItem(item pendingItem) *pendingItem {
	return &item
}
