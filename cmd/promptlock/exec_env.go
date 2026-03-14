package main

import (
	"os"

	"github.com/lunemec/promptlock/internal/app"
)

func buildLocalExecutionEnv(leased map[string]string) []string {
	return app.BuildExecutionEnvironment(os.Environ(), leased)
}
