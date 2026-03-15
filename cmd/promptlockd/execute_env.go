package main

import (
	"github.com/lunemec/promptlock/internal/app"
)

func buildBrokerExecutionEnv(ambient []string, leased map[string]string, pathOverride string) []string {
	return app.BuildExecutionEnvironmentWithPathOverride(ambient, leased, pathOverride)
}
