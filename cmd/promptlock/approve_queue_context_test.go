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
	if got := strings.Count(out.String(), "Request req-1 |"); got != 2 {
		t.Fatalf("expected req-1 to be shown twice across queue membership reset, got output %q", out.String())
	}
	if got := strings.Count(out.String(), "Request req-2 |"); got != 1 {
		t.Fatalf("expected req-2 to be shown once before queue reset, got output %q", out.String())
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

type stubWatchClient struct {
	snapshots    [][]pendingItem
	listCalls    int
	approveCalls int
	denyCalls    int
}

func (c *stubWatchClient) ListPending() ([]pendingItem, error) {
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
	return nil
}

func (c *stubWatchClient) Deny(string, string) error {
	c.denyCalls++
	return nil
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
		CommandFingerprint: "cmd-fp-1",
		WorkdirFingerprint: "wd-fp-1",
	}
}

func ptrPendingItem(item pendingItem) *pendingItem {
	return &item
}
