package main

import "github.com/lunemec/promptlock/internal/app"

func (s *server) controlPolicy() app.ControlPlanePolicy {
	if s.policyEngine != nil {
		return s.policyEngine
	}
	p := app.NewDefaultControlPlanePolicy(s.execPolicy, s.hostOpsPolicy, s.networkEgressPolicy)
	s.policyEngine = p
	return s.policyEngine
}
