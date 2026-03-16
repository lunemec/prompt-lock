package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/lunemec/promptlock/internal/adapters/audit"
)

func runAuditVerify(args []string) {
	fs := flag.NewFlagSet("audit-verify", flag.ExitOnError)
	auditPath := fs.String("file", "", "path to audit jsonl file")
	checkpoint := fs.String("checkpoint", "", "optional checkpoint file path")
	writeCheckpoint := fs.Bool("write-checkpoint", false, "write/refresh checkpoint with latest verified hash")
	fs.Parse(args)
	if strings.TrimSpace(*auditPath) == "" {
		fatal(fmt.Errorf("--file is required"))
	}
	if *checkpoint != "" {
		if prev, err := audit.ReadCheckpoint(*checkpoint); err == nil && strings.TrimSpace(prev) != "" {
			last, count, err := audit.VerifyFileAnchored(*auditPath, prev)
			if err != nil {
				fatal(err)
			}
			if *writeCheckpoint {
				if err := audit.WriteCheckpoint(*checkpoint, last); err != nil {
					fatal(err)
				}
			}
			fmt.Printf("audit verify ok: records=%d last_hash=%s\n", count, last)
			return
		}
	}
	last, count, err := audit.VerifyFile(*auditPath)
	if err != nil {
		fatal(err)
	}
	if *checkpoint != "" && *writeCheckpoint {
		if err := audit.WriteCheckpoint(*checkpoint, last); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("audit verify ok: records=%d last_hash=%s\n", count, last)
}
