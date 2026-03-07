package main

import (
	"strings"
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func FuzzValidateExecuteCommand(f *testing.F) {
	s := &server{execPolicy: config.ExecutionPolicy{
		AllowlistPrefixes: []string{"bash", "sh", "go", "python"},
		DenylistSubstrings: []string{"printenv", "/proc/", "environ"},
	}}
	f.Add("bash -lc echo ok")
	f.Add("python -c print(1)")
	f.Add("bash -lc printenv")

	f.Fuzz(func(t *testing.T, input string) {
		parts := strings.Fields(input)
		if len(parts) == 0 {
			parts = []string{"bash"}
		}
		_ = s.validateExecuteCommand(parts)
	})
}
