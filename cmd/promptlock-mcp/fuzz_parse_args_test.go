package main

import (
	"encoding/json"
	"testing"
)

func FuzzParseAndValidateExecArgs(f *testing.F) {
	seeds := []string{
		`{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}`,
		`{"intent":"x","command":["bash"],"ttl_minutes":1}`,
		`{"intent":"bad intent !","command":["bash"],"ttl_minutes":999}`,
		`{"intent":"run_tests","command":[],"ttl_minutes":5}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err != nil {
			return
		}
		_, _ = parseAndValidateExecArgs(m)
	})
}
