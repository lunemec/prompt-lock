package domain

import "testing"

func TestPolicyValidateRequest(t *testing.T) {
	p := DefaultPolicy()
	if err := p.ValidateRequest(5, []string{"a"}); err != nil {
		t.Fatalf("expected valid request: %v", err)
	}
	if err := p.ValidateRequest(0, []string{"a"}); err == nil {
		t.Fatalf("expected ttl validation failure")
	}
	if err := p.ValidateRequest(5, nil); err == nil {
		t.Fatalf("expected empty secrets failure")
	}
	if err := p.ValidateRequest(5, []string{"PATH"}); err == nil {
		t.Fatalf("expected reserved secret name failure")
	}
}
