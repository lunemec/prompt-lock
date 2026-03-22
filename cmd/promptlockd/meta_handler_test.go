package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestMetaCapabilitiesIncludesInsecureDevMode(t *testing.T) {
	s := &server{
		authEnabled: false,
		authCfg:     config.AuthConfig{AllowPlaintextSecretReturn: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/meta/capabilities", nil)
	w := httptest.NewRecorder()
	s.handleMetaCapabilities(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	v, ok := out["insecure_dev_mode"].(bool)
	if !ok {
		t.Fatalf("expected insecure_dev_mode bool field, got %#v", out["insecure_dev_mode"])
	}
	if !v {
		t.Fatalf("expected insecure_dev_mode=true")
	}
}

func TestMetaCapabilitiesInsecureDevModeFalseWhenAuthEnabled(t *testing.T) {
	s := &server{
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, AllowPlaintextSecretReturn: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/meta/capabilities", nil)
	w := httptest.NewRecorder()
	s.handleMetaCapabilities(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	v, ok := out["insecure_dev_mode"].(bool)
	if !ok {
		t.Fatalf("expected insecure_dev_mode bool field, got %#v", out["insecure_dev_mode"])
	}
	if v {
		t.Fatalf("expected insecure_dev_mode=false")
	}
}

func TestMetaCapabilitiesIncludesAgentBridgeAddressWhenConfigured(t *testing.T) {
	s := &server{
		authEnabled:        true,
		authCfg:            config.AuthConfig{EnableAuth: true, AllowPlaintextSecretReturn: false},
		agentBridgeAddress: "127.0.0.1:8766",
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/meta/capabilities", nil)
	w := httptest.NewRecorder()
	s.handleMetaCapabilities(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := out["agent_bridge_address"]; got != "127.0.0.1:8766" {
		t.Fatalf("agent_bridge_address = %#v, want 127.0.0.1:8766", got)
	}
}

func TestMetaCapabilitiesIncludesEnvPathEnabled(t *testing.T) {
	s := &server{
		authEnabled:    true,
		authCfg:        config.AuthConfig{EnableAuth: true, AllowPlaintextSecretReturn: false},
		envPathEnabled: true,
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/meta/capabilities", nil)
	w := httptest.NewRecorder()
	s.handleMetaCapabilities(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, ok := out["env_path_enabled"].(bool); !ok || !got {
		t.Fatalf("env_path_enabled = %#v, want true", out["env_path_enabled"])
	}
}
