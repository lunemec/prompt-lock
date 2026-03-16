package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lunemec/promptlock/internal/app"
)

func TestConfigureControlPlaneUseCasesInjectsAmbientEnvFromBoundary(t *testing.T) {
	s := &server{
		svc:               app.Service{Audit: testAudit{}},
		ambientProcessEnv: []string{"PATH=/boundary/bin", "HOME=/boundary-home"},
	}

	configureControlPlaneUseCases(s)

	if got, want := s.executeUseCase.AmbientEnv, []string{"PATH=/boundary/bin", "HOME=/boundary-home"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("execute use case ambient env = %#v, want %#v", got, want)
	}
	if got, want := s.hostDockerUseCase.AmbientEnv, []string{"PATH=/boundary/bin", "HOME=/boundary-home"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("host docker use case ambient env = %#v, want %#v", got, want)
	}
}

func TestConfigureControlPlaneUseCasesDoesNotFallBackToProcessEnvWhenBoundaryInjectionMissing(t *testing.T) {
	t.Setenv("PROMPTLOCK_ARCH006_SHOULD_NOT_LEAK", "ambient-secret")

	s := &server{
		svc: app.Service{Audit: testAudit{}},
	}

	configureControlPlaneUseCases(s)

	for _, env := range append(append([]string{}, s.executeUseCase.AmbientEnv...), s.hostDockerUseCase.AmbientEnv...) {
		if strings.HasPrefix(env, "PROMPTLOCK_ARCH006_SHOULD_NOT_LEAK=") {
			t.Fatalf("unexpected ambient env leak from process-global environment: %q", env)
		}
	}
	if len(s.executeUseCase.AmbientEnv) != 0 {
		t.Fatalf("execute use case ambient env = %#v, want empty without explicit boundary injection", s.executeUseCase.AmbientEnv)
	}
	if len(s.hostDockerUseCase.AmbientEnv) != 0 {
		t.Fatalf("host docker use case ambient env = %#v, want empty without explicit boundary injection", s.hostDockerUseCase.AmbientEnv)
	}
}
