package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lunemec/promptlock/internal/adapters/audit"
)

func runAuditVerify(args []string) {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("audit-verify", flag.ContinueOnError)
		fs.String("file", "", "path to audit jsonl file")
		fs.String("checkpoint", "", "optional checkpoint file path")
		fs.Bool("print-file", false, "print resolved audit jsonl file path and exit")
		fs.Bool("write-checkpoint", false, "write/refresh checkpoint with latest verified hash")
		printFlagHelp(os.Stdout, auditVerifyHelpText(), fs)
		return
	}
	fs := flag.NewFlagSet("audit-verify", flag.ExitOnError)
	auditPath := fs.String("file", "", "path to audit jsonl file")
	checkpoint := fs.String("checkpoint", "", "optional checkpoint file path")
	printFile := fs.Bool("print-file", false, "print resolved audit jsonl file path and exit")
	writeCheckpoint := fs.Bool("write-checkpoint", false, "write/refresh checkpoint with latest verified hash")
	fs.Parse(args)
	if strings.TrimSpace(*auditPath) == "" {
		*auditPath = defaultAuditVerifyPath()
	}
	if strings.TrimSpace(*auditPath) == "" {
		fatal(fmt.Errorf("--file is required (or run from a workspace with promptlock setup)"))
	}
	if *printFile {
		fmt.Println(*auditPath)
		return
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

func defaultAuditVerifyPath() string {
	cfg, ok := loadWorkspaceSetupConfig(resolvedWorkspaceSetupConfigPath(os.Getenv("PROMPTLOCK_CONFIG")))
	if !ok {
		return ""
	}
	return strings.TrimSpace(cfg.AuditPath)
}
