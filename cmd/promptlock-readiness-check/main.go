package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type task struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
	Blocking bool   `json:"blocking"`
}

type statusFile struct {
	Tasks []task `json:"tasks"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("promptlock-readiness-check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "docs/plans/status/PRODUCTION-READINESS-STATUS.json", "path to readiness status json")
	requireP0 := flags.Bool("require-p0", false, "legacy name: fail when any release-gating task (explicit P0 or blocking=true) is not done")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	b, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(stderr, "read status file: %v\n", err)
		return 2
	}
	var st statusFile
	if err := json.Unmarshal(b, &st); err != nil {
		fmt.Fprintf(stderr, "parse status file: %v\n", err)
		return 2
	}

	if !*requireP0 {
		fmt.Fprintln(stdout, "readiness status loaded")
		return 0
	}
	if err := validateReleaseGatingStatus(st); err != nil {
		fmt.Fprintf(stderr, "validate status file: %v\n", err)
		return 2
	}

	open := []string{}
	for _, t := range st.Tasks {
		if !isReleaseGatingTask(t) {
			continue
		}
		if !isDoneStatus(t.Status) {
			open = append(open, t.ID+":"+t.Status)
		}
	}

	if len(open) > 0 {
		fmt.Fprintf(stderr, "production readiness gate failed: open release-gating tasks: %s\n", strings.Join(open, ", "))
		return 1
	}
	fmt.Fprintln(stdout, "production readiness gate passed: all release-gating tasks done")
	return 0
}

func validateReleaseGatingStatus(st statusFile) error {
	if st.Tasks == nil {
		return fmt.Errorf("tasks array is required")
	}
	if len(st.Tasks) == 0 {
		return fmt.Errorf("at least one release-gating task is required")
	}
	for _, t := range st.Tasks {
		if isReleaseGatingTask(t) {
			return nil
		}
	}
	return fmt.Errorf("at least one release-gating task is required")
}

func isReleaseGatingTask(t task) bool {
	priority := strings.TrimSpace(t.Priority)
	id := strings.TrimSpace(t.ID)
	return strings.EqualFold(priority, "P0") || strings.HasPrefix(strings.ToUpper(id), "P0-") || t.Blocking
}

func isDoneStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "done")
}
