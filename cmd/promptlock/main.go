package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, promptlockHelpText())
		os.Exit(2)
	}
	switch os.Args[1] {
	case "-h", "--help":
		fmt.Print(promptlockHelpText())
		return
	}
	switch os.Args[1] {
	case "exec":
		runExec(os.Args[2:])
	case "watch":
		runWatch(os.Args[2:])
	case "audit-verify":
		runAuditVerify(os.Args[2:])
	case "auth":
		runAuth(os.Args[2:])
	case "setup":
		runSetup(os.Args[2:])
	case "help":
		fmt.Print(promptlockHelpText())
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		fmt.Fprint(os.Stderr, promptlockHelpText())
		os.Exit(2)
	}
}
