package sopsenv

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLoadFromFileNoPathNoop(t *testing.T) {
	orig := decryptFile
	t.Cleanup(func() { decryptFile = orig })
	called := false
	decryptFile = func(_ string) ([]byte, error) {
		called = true
		return nil, nil
	}

	if err := LoadFromFile("", nil); err != nil {
		t.Fatalf("expected nil error for empty path, got %v", err)
	}
	if called {
		t.Fatalf("decrypt should not be called when path is empty")
	}
}

func TestLoadFromFileLoadsJSONAndPreservesExistingEnv(t *testing.T) {
	orig := decryptFile
	t.Cleanup(func() { decryptFile = orig })

	existingKey := "PROMPTLOCK_SOPSENV_TEST_EXISTING"
	newKey := "PROMPTLOCK_SOPSENV_TEST_NEW"
	if err := os.Setenv(existingKey, "keep"); err != nil {
		t.Fatalf("set existing env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(existingKey)
		_ = os.Unsetenv(newKey)
	})

	decryptFile = func(_ string) ([]byte, error) {
		return []byte(fmt.Sprintf(`{"%s":"override","%s":"value"}`, existingKey, newKey)), nil
	}

	if err := LoadFromFile("/tmp/keys.sops.json", []string{newKey}); err != nil {
		t.Fatalf("load from file: %v", err)
	}
	if got := os.Getenv(existingKey); got != "keep" {
		t.Fatalf("expected existing env to be preserved, got %q", got)
	}
	if got := os.Getenv(newKey); got != "value" {
		t.Fatalf("expected new key from sops payload, got %q", got)
	}
}

func TestLoadFromFileParsesDotenvPayload(t *testing.T) {
	orig := decryptFile
	t.Cleanup(func() { decryptFile = orig })

	keyA := "PROMPTLOCK_SOPSENV_TEST_A"
	keyB := "PROMPTLOCK_SOPSENV_TEST_B"
	t.Cleanup(func() {
		_ = os.Unsetenv(keyA)
		_ = os.Unsetenv(keyB)
	})

	decryptFile = func(_ string) ([]byte, error) {
		return []byte("# comment\n" + keyA + "=one\n" + keyB + "=\"two\"\n"), nil
	}

	if err := LoadFromFile("/tmp/keys.sops.env", []string{keyA, keyB}); err != nil {
		t.Fatalf("load dotenv payload: %v", err)
	}
	if got := os.Getenv(keyA); got != "one" {
		t.Fatalf("unexpected %s: %q", keyA, got)
	}
	if got := os.Getenv(keyB); got != "two" {
		t.Fatalf("unexpected %s: %q", keyB, got)
	}
}

func TestLoadFromFileFailsWhenRequiredKeyMissing(t *testing.T) {
	orig := decryptFile
	t.Cleanup(func() { decryptFile = orig })

	decryptFile = func(_ string) ([]byte, error) {
		return []byte(`{"PROMPTLOCK_SOPSENV_TEST_ONLY":"x"}`), nil
	}

	err := LoadFromFile("/tmp/keys.sops.json", []string{"PROMPTLOCK_SOPSENV_TEST_MISSING"})
	if err == nil {
		t.Fatalf("expected required-key error")
	}
	if !strings.Contains(err.Error(), "required env") {
		t.Fatalf("expected required env error, got %v", err)
	}
}

func TestLoadFromFileFailsOnNonStringJSONValue(t *testing.T) {
	orig := decryptFile
	t.Cleanup(func() { decryptFile = orig })

	decryptFile = func(_ string) ([]byte, error) {
		return []byte(`{"PROMPTLOCK_SOPSENV_TEST_NUM":1}`), nil
	}

	err := LoadFromFile("/tmp/keys.sops.json", nil)
	if err == nil {
		t.Fatalf("expected json value type error")
	}
	if !strings.Contains(err.Error(), "must be string") {
		t.Fatalf("expected json value type error, got %v", err)
	}
}

func TestLoadFromFilePropagatesDecryptError(t *testing.T) {
	orig := decryptFile
	t.Cleanup(func() { decryptFile = orig })

	decryptFile = func(_ string) ([]byte, error) {
		return nil, fmt.Errorf("boom")
	}

	err := LoadFromFile("/tmp/keys.sops.json", nil)
	if err == nil {
		t.Fatalf("expected decrypt error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected decrypt error details, got %v", err)
	}
}
