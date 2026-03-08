package main

import (
	"encoding/json"
	"flag"
	"fmt"
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
	file := flag.String("file", "docs/plans/PRODUCTION-READINESS-STATUS.json", "path to readiness status json")
	requireP0 := flag.Bool("require-p0", false, "fail when any P0 task is not done")
	flag.Parse()

	b, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read status file: %v\n", err)
		os.Exit(2)
	}
	var st statusFile
	if err := json.Unmarshal(b, &st); err != nil {
		fmt.Fprintf(os.Stderr, "parse status file: %v\n", err)
		os.Exit(2)
	}

	if !*requireP0 {
		fmt.Println("readiness status loaded")
		return
	}

	open := []string{}
	for _, t := range st.Tasks {
		isP0 := strings.EqualFold(strings.TrimSpace(t.Priority), "P0") || strings.HasPrefix(strings.ToUpper(strings.TrimSpace(t.ID)), "P0-") || t.Blocking
		if !isP0 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(t.Status), "done") {
			open = append(open, t.ID+":"+t.Status)
		}
	}

	if len(open) > 0 {
		fmt.Fprintf(os.Stderr, "production readiness gate failed: open P0 tasks: %s\n", strings.Join(open, ", "))
		os.Exit(1)
	}
	fmt.Println("production readiness gate passed: all P0 tasks done")
}
