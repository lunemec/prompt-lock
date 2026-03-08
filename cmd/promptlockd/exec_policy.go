package main

import "github.com/lunemec/promptlock/internal/app"

func (s *server) validateExecuteRequest(req executeReq) error {
	return s.controlPolicy().ValidateExecuteRequest(s.securityProfile, app.ExecuteRequest{Intent: req.Intent, Command: req.Command})
}

func (s *server) validateExecuteCommand(cmd []string) error {
	return s.controlPolicy().ValidateExecuteCommand(cmd)
}

func applyOutputSecurity(mode, in string) string {
	engine := app.DefaultControlPlanePolicy{OutputMode: mode}
	return engine.ApplyOutputSecurity(in)
}

func redactOutput(in string) string {
	engine := app.DefaultControlPlanePolicy{OutputMode: "redacted"}
	return engine.ApplyOutputSecurity(in)
}
