package fsyncreport

import (
	"bytes"
	"strings"
	"testing"
)

func TestComputeSignatureIgnoresSignatureField(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-03-09T00:00:00Z",
		GeneratedBy:   GeneratedBy,
		Hostname:      "runner",
		OK:            true,
		Results: []Result{
			{Dir: "/var/lib/promptlock", OK: true},
		},
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	sigA, err := ComputeSignature(report, key)
	if err != nil {
		t.Fatalf("compute signature: %v", err)
	}
	report.Signature = Signature{Alg: SignatureAlgHMACSHA256, KeyID: "k1", Value: "tampered"}
	sigB, err := ComputeSignature(report, key)
	if err != nil {
		t.Fatalf("compute signature with signature field present: %v", err)
	}
	if !bytes.Equal(sigA, sigB) {
		t.Fatalf("expected deterministic payload to exclude signature field")
	}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-03-09T00:00:00Z",
		GeneratedBy:   GeneratedBy,
		Hostname:      "runner",
		OK:            true,
		Results: []Result{
			{Dir: "/var/lib/promptlock", OK: true},
			{Dir: "/var/log/promptlock", OK: true},
		},
	}
	material := KeyMaterial{
		Key:   []byte("0123456789abcdef0123456789abcdef"),
		KeyID: "release-key-1",
	}
	if err := SignReport(&report, material); err != nil {
		t.Fatalf("sign report: %v", err)
	}
	if report.Signature.Value == "" {
		t.Fatalf("expected signature value")
	}
	if err := VerifyReportSignature(report, material); err != nil {
		t.Fatalf("verify report signature: %v", err)
	}
}

func TestVerifyReportSignatureFailsOnKeyIDMismatch(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-03-09T00:00:00Z",
		GeneratedBy:   GeneratedBy,
		Hostname:      "runner",
		OK:            true,
		Results: []Result{
			{Dir: "/var/lib/promptlock", OK: true},
		},
	}
	material := KeyMaterial{
		Key:   []byte("0123456789abcdef0123456789abcdef"),
		KeyID: "release-key-1",
	}
	if err := SignReport(&report, material); err != nil {
		t.Fatalf("sign report: %v", err)
	}
	if err := VerifyReportSignature(report, KeyMaterial{Key: material.Key, KeyID: "release-key-2"}); err == nil {
		t.Fatalf("expected key-id mismatch verification error")
	}
}

func TestResolveVerificationKeyringFromEnvIncludesPrimaryAndRotatedKeys(t *testing.T) {
	t.Setenv(DefaultHMACKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(DefaultHMACKeyIDEnv, "release-key-current")
	t.Setenv(DefaultHMACKeyringEnv, "release-key-prev:PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV")
	t.Setenv("PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV", "fedcba9876543210fedcba9876543210")

	keyring, err := ResolveVerificationKeyringFromEnv("", "", "")
	if err != nil {
		t.Fatalf("resolve verification keyring: %v", err)
	}
	if keyring.PrimaryKeyID != "release-key-current" {
		t.Fatalf("expected primary key id release-key-current, got %q", keyring.PrimaryKeyID)
	}
	if len(keyring.Keys) != 2 {
		t.Fatalf("expected 2 verification keys, got %d", len(keyring.Keys))
	}
	if _, ok := keyring.Keys["release-key-current"]; !ok {
		t.Fatalf("expected primary key in keyring")
	}
	if _, ok := keyring.Keys["release-key-prev"]; !ok {
		t.Fatalf("expected rotated key in keyring")
	}
}

func TestResolveVerificationKeyringFromEnvFailsOnDuplicateKeyID(t *testing.T) {
	t.Setenv(DefaultHMACKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(DefaultHMACKeyIDEnv, "release-key-current")
	t.Setenv(DefaultHMACKeyringEnv, "release-key-current:PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV")
	t.Setenv("PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV", "fedcba9876543210fedcba9876543210")

	_, err := ResolveVerificationKeyringFromEnv("", "", "")
	if err == nil {
		t.Fatalf("expected duplicate key-id resolution failure")
	}
	if !strings.Contains(err.Error(), "duplicate key_id") {
		t.Fatalf("expected duplicate key_id error, got %v", err)
	}
}

func TestResolveVerificationKeyringFromEnvFailsWhenEntryMalformed(t *testing.T) {
	t.Setenv(DefaultHMACKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(DefaultHMACKeyIDEnv, "release-key-current")
	t.Setenv(DefaultHMACKeyringEnv, "release-key-prev")

	_, err := ResolveVerificationKeyringFromEnv("", "", "")
	if err == nil {
		t.Fatalf("expected malformed keyring entry failure")
	}
	if !strings.Contains(err.Error(), "expected <key_id>:<env_var_name>") {
		t.Fatalf("expected malformed keyring entry error, got %v", err)
	}
}

func TestResolveVerificationKeyringFromEnvFailsWhenKeyEnvMissing(t *testing.T) {
	t.Setenv(DefaultHMACKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(DefaultHMACKeyIDEnv, "release-key-current")
	t.Setenv(DefaultHMACKeyringEnv, "release-key-prev:PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV")
	t.Setenv("PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV", "")

	_, err := ResolveVerificationKeyringFromEnv("", "", "")
	if err == nil {
		t.Fatalf("expected missing key env failure")
	}
	if !strings.Contains(err.Error(), "missing hmac key env") {
		t.Fatalf("expected missing key env error, got %v", err)
	}
}

func TestVerifyReportSignatureWithKeyringAcceptsRotatedKey(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-03-09T00:00:00Z",
		GeneratedBy:   GeneratedBy,
		Hostname:      "runner",
		OK:            true,
		Results: []Result{
			{Dir: "/var/lib/promptlock", OK: true},
		},
	}
	rotatedKey := []byte("fedcba9876543210fedcba9876543210")
	if err := SignReport(&report, KeyMaterial{Key: rotatedKey, KeyID: "release-key-prev"}); err != nil {
		t.Fatalf("sign with rotated key: %v", err)
	}

	keyring := VerificationKeyring{
		PrimaryKeyID: "release-key-current",
		Keys: map[string][]byte{
			"release-key-current": []byte("0123456789abcdef0123456789abcdef"),
			"release-key-prev":    rotatedKey,
		},
	}
	if err := VerifyReportSignatureWithKeyring(report, keyring); err != nil {
		t.Fatalf("verify rotated signature with keyring: %v", err)
	}
}

func TestVerifyReportSignatureWithKeyringFailsOnUnknownKeyID(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-03-09T00:00:00Z",
		GeneratedBy:   GeneratedBy,
		Hostname:      "runner",
		OK:            true,
		Results: []Result{
			{Dir: "/var/lib/promptlock", OK: true},
		},
	}
	if err := SignReport(&report, KeyMaterial{
		Key:   []byte("fedcba9876543210fedcba9876543210"),
		KeyID: "release-key-prev",
	}); err != nil {
		t.Fatalf("sign with rotated key: %v", err)
	}

	keyring := VerificationKeyring{
		PrimaryKeyID: "release-key-current",
		Keys: map[string][]byte{
			"release-key-current": []byte("0123456789abcdef0123456789abcdef"),
		},
	}
	if err := VerifyReportSignatureWithKeyring(report, keyring); err == nil {
		t.Fatalf("expected unknown key-id verification error")
	}
}

func TestVerifyReportSignatureWithKeyringFailsWhenSignatureKeyIDHasWhitespace(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-03-09T00:00:00Z",
		GeneratedBy:   GeneratedBy,
		Hostname:      "runner",
		OK:            true,
		Results: []Result{
			{Dir: "/var/lib/promptlock", OK: true},
		},
	}
	if err := SignReport(&report, KeyMaterial{
		Key:   []byte("0123456789abcdef0123456789abcdef"),
		KeyID: "release-key-current",
	}); err != nil {
		t.Fatalf("sign report: %v", err)
	}
	report.Signature.KeyID = " release-key-current "

	keyring := VerificationKeyring{
		PrimaryKeyID: "release-key-current",
		Keys: map[string][]byte{
			"release-key-current": []byte("0123456789abcdef0123456789abcdef"),
		},
	}
	if err := VerifyReportSignatureWithKeyring(report, keyring); err == nil {
		t.Fatalf("expected signature key-id whitespace validation error")
	}
}
