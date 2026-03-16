package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

func commandFingerprint(cmd []string) string {
	s := strings.Join(cmd, "\x00")
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func workdirFingerprint() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(wd))
	return hex.EncodeToString(h[:]), nil
}

func detectRiskyCommand(cmd []string) string {
	joined := strings.ToLower(strings.Join(cmd, " "))
	risky := []string{"printenv", " env", "/proc/", "environ", "set "}
	for _, r := range risky {
		if strings.Contains(joined, r) {
			return fmt.Sprintf("contains risky pattern %q", strings.TrimSpace(r))
		}
	}
	return ""
}

func writeJSONStdout(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}
