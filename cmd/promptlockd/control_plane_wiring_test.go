package main

import (
	"reflect"
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
