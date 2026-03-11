package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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

func TestRunApproveListDisplaysDecisionContext(t *testing.T) {
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
		runApproveList([]string{"--broker", ts.URL, "--operator-token", "op-token"})
	})

	checks := []string{
		"req-ctx-1",
		"agent=agent-1",
		"task=task-1",
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
