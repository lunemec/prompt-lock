package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name            string
		args            []string
		filePath        func(t *testing.T) string
		wantCode        int
		wantStdout      string
		wantStderr      string
		wantStderrEmpty bool
	}

	writeStatusFile := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "status.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write status file: %v", err)
		}
		return path
	}

	tests := []testCase{
		{
			name: "missing file",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.json")
			},
			wantCode:   2,
			wantStderr: "read status file:",
		},
		{
			name: "unreadable path",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return t.TempDir()
			},
			wantCode:   2,
			wantStderr: "read status file:",
		},
		{
			name: "malformed json",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, "{")
			},
			wantCode:   2,
			wantStderr: "parse status file:",
		},
		{
			name: "missing tasks fails closed",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{}`)
			},
			wantCode:   2,
			wantStderr: "validate status file: tasks array is required",
		},
		{
			name: "empty tasks fails closed",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{"tasks":[]}`)
			},
			wantCode:   2,
			wantStderr: "validate status file: at least one release-gating task is required",
		},
		{
			name: "schema light tasks fail closed",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{"tasks":[{"status":"done"},{"id":"P2-01","status":"done"}]}`)
			},
			wantCode:   2,
			wantStderr: "validate status file: at least one release-gating task is required",
		},
		{
			name: "load only when require p0 disabled",
			args: nil,
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{"tasks":[{"id":"P0-01","priority":"P0","status":"open","blocking":true}]}`)
			},
			wantCode:        0,
			wantStdout:      "readiness status loaded\n",
			wantStderrEmpty: true,
		},
		{
			name: "p0 done passes",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{"tasks":[{"id":"P0-01","priority":"P0","status":"done","blocking":true},{"id":"P2-01","priority":"P2","status":"open","blocking":false}]}`)
			},
			wantCode:        0,
			wantStdout:      "production readiness gate passed: all release-gating tasks done\n",
			wantStderrEmpty: true,
		},
		{
			name: "p0 open fails",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{"tasks":[{"id":"P0-01","priority":"P0","status":"open","blocking":true}]}`)
			},
			wantCode:   1,
			wantStderr: "production readiness gate failed: open release-gating tasks: P0-01:open",
		},
		{
			name: "non p0 blocking task fails",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{"tasks":[{"id":"P2-01","priority":"P2","status":"open","blocking":true}]}`)
			},
			wantCode:   1,
			wantStderr: "production readiness gate failed: open release-gating tasks: P2-01:open",
		},
		{
			name: "case and whitespace normalization",
			args: []string{"--require-p0"},
			filePath: func(t *testing.T) string {
				return writeStatusFile(t, `{"tasks":[{"id":" p0-01 ","priority":" p2 ","status":" Done ","blocking":false},{"id":" p2-02 ","priority":" p0 ","status":" done ","blocking":false},{"id":" p2-03 ","priority":" p2 ","status":" DONE ","blocking":true}]}`)
			},
			wantCode:        0,
			wantStdout:      "production readiness gate passed: all release-gating tasks done\n",
			wantStderrEmpty: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file := tt.filePath(t)
			args := append([]string{"--file", file}, tt.args...)

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := run(args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d", code, tt.wantCode)
			}
			if stdout.String() != tt.wantStdout {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderrEmpty {
				if stderr.Len() != 0 {
					t.Fatalf("stderr = %q, want empty", stderr.String())
				}
				return
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
