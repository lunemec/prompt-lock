package fsyncreport

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	SchemaVersion                  = "v1"
	GeneratedBy                    = "promptlock-storage-fsync-check"
	SignatureAlgHMACSHA256         = "hmac-sha256"
	DefaultHMACKeyEnv              = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY"
	DefaultHMACKeyIDEnv            = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID"
	DefaultHMACKeyringEnv          = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING"
	DefaultHMACKeyOverlapMaxAgeEnv = "PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE"
	minHMACKeyLength               = 32
)

type Result struct {
	Dir   string `json:"dir"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type Signature struct {
	Alg   string `json:"alg"`
	KeyID string `json:"key_id"`
	Value string `json:"value"`
}

type Report struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   string    `json:"generated_at"`
	GeneratedBy   string    `json:"generated_by"`
	Hostname      string    `json:"hostname"`
	OK            bool      `json:"ok"`
	Results       []Result  `json:"results"`
	Signature     Signature `json:"signature"`
}

type signingPayload struct {
	SchemaVersion string   `json:"schema_version"`
	GeneratedAt   string   `json:"generated_at"`
	GeneratedBy   string   `json:"generated_by"`
	Hostname      string   `json:"hostname"`
	OK            bool     `json:"ok"`
	Results       []Result `json:"results"`
}

type KeyMaterial struct {
	Key   []byte
	KeyID string
}

type VerificationKeyring struct {
	PrimaryKeyID string
	Keys         map[string][]byte
}

func resolveEnvName(raw string, fallback string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return v
	}
	return fallback
}

func normalizeHMACKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < minHMACKeyLength {
		return nil, fmt.Errorf("hmac key must be at least %d characters", minHMACKeyLength)
	}
	return []byte(trimmed), nil
}

func ResolveKeyMaterialFromEnv(keyEnv string, keyIDEnv string) (KeyMaterial, error) {
	resolvedKeyEnv := resolveEnvName(keyEnv, DefaultHMACKeyEnv)
	resolvedKeyIDEnv := resolveEnvName(keyIDEnv, DefaultHMACKeyIDEnv)

	rawKey := os.Getenv(resolvedKeyEnv)
	if strings.TrimSpace(rawKey) == "" {
		return KeyMaterial{}, fmt.Errorf("missing hmac key env %s", resolvedKeyEnv)
	}
	key, err := normalizeHMACKey(rawKey)
	if err != nil {
		return KeyMaterial{}, fmt.Errorf("invalid hmac key env %s: %w", resolvedKeyEnv, err)
	}

	keyID := strings.TrimSpace(os.Getenv(resolvedKeyIDEnv))
	if keyID == "" {
		return KeyMaterial{}, fmt.Errorf("missing hmac key id env %s", resolvedKeyIDEnv)
	}

	return KeyMaterial{Key: key, KeyID: keyID}, nil
}

func ResolveVerificationKeyringFromEnv(keyEnv string, keyIDEnv string, keyringEnv string) (VerificationKeyring, error) {
	primary, err := ResolveKeyMaterialFromEnv(keyEnv, keyIDEnv)
	if err != nil {
		return VerificationKeyring{}, err
	}

	keyring := VerificationKeyring{
		PrimaryKeyID: strings.TrimSpace(primary.KeyID),
		Keys: map[string][]byte{
			strings.TrimSpace(primary.KeyID): append([]byte(nil), primary.Key...),
		},
	}

	resolvedKeyringEnv := resolveEnvName(keyringEnv, DefaultHMACKeyringEnv)
	rawSpec := strings.TrimSpace(os.Getenv(resolvedKeyringEnv))
	if rawSpec == "" {
		return keyring, nil
	}

	entries := strings.Split(rawSpec, ",")
	for idx, rawEntry := range entries {
		entry := strings.TrimSpace(rawEntry)
		if entry == "" {
			return VerificationKeyring{}, fmt.Errorf("invalid keyring entry at index %d in env %s", idx, resolvedKeyringEnv)
		}

		keyIDPart, keyEnvNamePart, ok := strings.Cut(entry, ":")
		if !ok {
			return VerificationKeyring{}, fmt.Errorf("invalid keyring entry %q in env %s: expected <key_id>:<env_var_name>", entry, resolvedKeyringEnv)
		}
		keyID := strings.TrimSpace(keyIDPart)
		keyEnvName := strings.TrimSpace(keyEnvNamePart)
		if keyID == "" || keyEnvName == "" {
			return VerificationKeyring{}, fmt.Errorf("invalid keyring entry %q in env %s: key_id and env_var_name are required", entry, resolvedKeyringEnv)
		}
		if _, exists := keyring.Keys[keyID]; exists {
			return VerificationKeyring{}, fmt.Errorf("duplicate key_id %q in verification keyring", keyID)
		}

		rawKey := os.Getenv(keyEnvName)
		if strings.TrimSpace(rawKey) == "" {
			return VerificationKeyring{}, fmt.Errorf("missing hmac key env %s for key_id %q", keyEnvName, keyID)
		}
		key, err := normalizeHMACKey(rawKey)
		if err != nil {
			return VerificationKeyring{}, fmt.Errorf("invalid hmac key env %s for key_id %q: %w", keyEnvName, keyID, err)
		}
		keyring.Keys[keyID] = key
	}

	return keyring, nil
}

func signingPayloadFromReport(report Report) signingPayload {
	results := make([]Result, len(report.Results))
	copy(results, report.Results)
	return signingPayload{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   report.GeneratedAt,
		GeneratedBy:   report.GeneratedBy,
		Hostname:      report.Hostname,
		OK:            report.OK,
		Results:       results,
	}
}

func SigningPayload(report Report) ([]byte, error) {
	return json.Marshal(signingPayloadFromReport(report))
}

func ComputeSignature(report Report, key []byte) ([]byte, error) {
	payload, err := SigningPayload(report)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(payload); err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}

func validateKeyMaterial(material KeyMaterial) error {
	if len(material.Key) < minHMACKeyLength {
		return fmt.Errorf("hmac key must be at least %d characters", minHMACKeyLength)
	}
	if strings.TrimSpace(material.KeyID) == "" {
		return fmt.Errorf("hmac key id is required")
	}
	return nil
}

func SignReport(report *Report, material KeyMaterial) error {
	if report == nil {
		return fmt.Errorf("report is required")
	}
	if err := validateKeyMaterial(material); err != nil {
		return err
	}
	sum, err := ComputeSignature(*report, material.Key)
	if err != nil {
		return err
	}
	report.Signature = Signature{
		Alg:   SignatureAlgHMACSHA256,
		KeyID: strings.TrimSpace(material.KeyID),
		Value: base64.StdEncoding.EncodeToString(sum),
	}
	return nil
}

func validateVerificationKeyring(keyring VerificationKeyring) error {
	if strings.TrimSpace(keyring.PrimaryKeyID) == "" {
		return fmt.Errorf("primary key id is required")
	}
	if len(keyring.Keys) == 0 {
		return fmt.Errorf("verification keyring must include at least one key")
	}
	primaryKeyID := strings.TrimSpace(keyring.PrimaryKeyID)
	primaryKey, ok := keyring.Keys[primaryKeyID]
	if !ok {
		return fmt.Errorf("verification keyring missing primary key id %q", primaryKeyID)
	}
	if len(primaryKey) < minHMACKeyLength {
		return fmt.Errorf("hmac key must be at least %d characters", minHMACKeyLength)
	}
	for keyID, key := range keyring.Keys {
		trimmedKeyID := strings.TrimSpace(keyID)
		if trimmedKeyID == "" {
			return fmt.Errorf("verification keyring contains empty key id")
		}
		if trimmedKeyID != keyID {
			return fmt.Errorf("verification keyring key id %q must not include leading or trailing whitespace", keyID)
		}
		if len(key) < minHMACKeyLength {
			return fmt.Errorf("hmac key for key_id %q must be at least %d characters", keyID, minHMACKeyLength)
		}
	}
	return nil
}

func VerifyReportSignatureWithKeyring(report Report, keyring VerificationKeyring) error {
	if err := validateVerificationKeyring(keyring); err != nil {
		return err
	}
	sig := report.Signature

	if strings.TrimSpace(sig.Alg) == "" {
		return fmt.Errorf("signature.alg is required")
	}
	if sig.Alg != SignatureAlgHMACSHA256 {
		return fmt.Errorf("signature.alg must be %q", SignatureAlgHMACSHA256)
	}
	if strings.TrimSpace(sig.KeyID) == "" {
		return fmt.Errorf("signature.key_id is required")
	}
	if strings.TrimSpace(sig.Value) == "" {
		return fmt.Errorf("signature.value is required")
	}

	keyID := strings.TrimSpace(sig.KeyID)
	if keyID != sig.KeyID {
		return fmt.Errorf("signature.key_id must not include leading or trailing whitespace")
	}
	key, ok := keyring.Keys[keyID]
	if !ok {
		return fmt.Errorf("signature.key_id %q is not present in verification keyring", keyID)
	}

	actual, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return fmt.Errorf("signature.value must be base64: %w", err)
	}
	expected, err := ComputeSignature(report, key)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func VerifyReportSignature(report Report, material KeyMaterial) error {
	if err := validateKeyMaterial(material); err != nil {
		return err
	}
	keyID := strings.TrimSpace(material.KeyID)
	return VerifyReportSignatureWithKeyring(report, VerificationKeyring{
		PrimaryKeyID: keyID,
		Keys: map[string][]byte{
			keyID: append([]byte(nil), material.Key...),
		},
	})
}
