package main

import (
	"os"

	"github.com/lunemec/promptlock/internal/app"
)

func buildBrokerExecutionEnv(leased map[string]string, pathOverride string) []string {
	return app.BuildExecutionEnvironmentWithPathOverride(os.Environ(), leased, pathOverride)
}
