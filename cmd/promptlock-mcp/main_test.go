package main

import "testing"

func TestParseAndValidateExecArgs(t *testing.T) {
	ok, err := parseAndValidateExecArgs(map[string]interface{}{
		"intent":      "run_tests",
		"command":     []interface{}{"bash", "-lc", "echo ok"},
		"ttl_minutes": float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok.Intent != "run_tests" || len(ok.Cmd) != 3 || ok.TTL != 5 {
		t.Fatalf("unexpected parsed args: %#v", ok)
	}

	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "", "command": []interface{}{"bash"}}); err == nil {
		t.Fatalf("expected invalid intent")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{}, "ttl_minutes": float64(5)}); err == nil {
		t.Fatalf("expected invalid command")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{"bash", float64(1)}, "ttl_minutes": float64(5)}); err == nil {
		t.Fatalf("expected invalid non-string command argument")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{"bash"}, "ttl_minutes": float64(999)}); err == nil {
		t.Fatalf("expected invalid ttl")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{"bash"}, "ttl_minutes": float64(1.5)}); err == nil {
		t.Fatalf("expected invalid non-integer ttl")
	}
}
