package main

import (
	"os"

	"github.com/lunemec/promptlock/internal/app"
)

func configureControlPlaneUseCases(s *server) {
	if s == nil {
		return
	}
	if s.policyEngine == nil {
		s.policyEngine = app.NewDefaultControlPlanePolicy(s.execPolicy, s.hostOpsPolicy, s.networkEgressPolicy)
	}
	if s.svc.AuditFailureHandler == nil {
		s.svc.AuditFailureHandler = func(err error) error {
			return s.closeDurabilityGate("audit", err)
		}
	}
	ambientEnv := boundaryAmbientEnv(s.ambientProcessEnv)
	runner := processRunner{}
	s.executeUseCase = app.ExecuteWithLeaseUseCase{
		Service:             &s.svc,
		Policy:              s.policyEngine,
		SecurityProfile:     s.securityProfile,
		Runner:              runner,
		MaxOutputBytes:      s.execPolicy.MaxOutputBytes,
		OutputMode:          s.execPolicy.OutputSecurityMode,
		AmbientEnv:          ambientEnv,
		AuditFailureWarning: durabilityUnavailableMessage,
	}
	s.hostDockerUseCase = app.HostDockerExecuteUseCase{
		Service:             &s.svc,
		Policy:              s.policyEngine,
		Runner:              runner,
		MaxOutputBytes:      s.execPolicy.MaxOutputBytes,
		OutputMode:          "redacted",
		AmbientEnv:          ambientEnv,
		TimeoutSec:          s.hostOpsPolicy.DockerTimeoutSec,
		AuditFailureWarning: durabilityUnavailableMessage,
	}
}

func boundaryAmbientEnv(explicit []string) []string {
	if explicit != nil {
		return append([]string(nil), explicit...)
	}
	return os.Environ()
}
