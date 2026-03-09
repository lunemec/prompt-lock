package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type persistedState struct {
	Bootstrap map[string]BootstrapToken `json:"bootstrap"`
	Grants    map[string]PairingGrant   `json:"grants"`
	Sessions  map[string]SessionToken   `json:"sessions"`
}

type encryptedPersistedState struct {
	Version    string `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

const encryptedPersistedStateV1 = "v1"

var syncParentDir = func(path string) error {
	dir := filepath.Dir(path)
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open parent dir %s: %w", dir, err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("sync parent dir %s: %w", dir, err)
	}
	return nil
}

func (s *Store) snapshotState() persistedState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := persistedState{
		Bootstrap: make(map[string]BootstrapToken, len(s.bootstrap)),
		Grants:    make(map[string]PairingGrant, len(s.grants)),
		Sessions:  make(map[string]SessionToken, len(s.sessions)),
	}
	for k, v := range s.bootstrap {
		out.Bootstrap[k] = v
	}
	for k, v := range s.grants {
		out.Grants[k] = v
	}
	for k, v := range s.sessions {
		out.Sessions[k] = v
	}
	return out
}

func (s *Store) restoreState(state persistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.Bootstrap != nil {
		s.bootstrap = state.Bootstrap
	}
	if state.Grants != nil {
		s.grants = state.Grants
	}
	if state.Sessions != nil {
		s.sessions = state.Sessions
	}
}

func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if err := syncParentDir(path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func normalizeEncryptionKey(key []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(key))
	if len(trimmed) < 16 {
		return nil, fmt.Errorf("encryption key must be at least 16 characters")
	}
	sum := sha256.Sum256([]byte(trimmed))
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out, nil
}

func encryptState(plaintext []byte, key []byte) (encryptedPersistedState, error) {
	normalizedKey, err := normalizeEncryptionKey(key)
	if err != nil {
		return encryptedPersistedState{}, err
	}
	block, err := aes.NewCipher(normalizedKey)
	if err != nil {
		return encryptedPersistedState{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedPersistedState{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encryptedPersistedState{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return encryptedPersistedState{
		Version:    encryptedPersistedStateV1,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptState(in encryptedPersistedState, key []byte) ([]byte, error) {
	if strings.TrimSpace(in.Version) != encryptedPersistedStateV1 {
		return nil, fmt.Errorf("unsupported encrypted auth store version %q", in.Version)
	}
	normalizedKey, err := normalizeEncryptionKey(key)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(in.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(in.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher(normalizedKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt auth store: %w", err)
	}
	return plaintext, nil
}

func (s *Store) SaveToFile(path string) error {
	state := s.snapshotState()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomically(path, b); err != nil {
		return err
	}
	return nil
}

func (s *Store) SaveToFileEncrypted(path string, key []byte) error {
	state := s.snapshotState()
	rawState, err := json.Marshal(state)
	if err != nil {
		return err
	}
	enc, err := encryptState(rawState, key)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(enc, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomically(path, b); err != nil {
		return err
	}
	return nil
}

func (s *Store) LoadFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var state persistedState
	if err := json.Unmarshal(b, &state); err != nil {
		return fmt.Errorf("parse auth store: %w", err)
	}
	s.restoreState(state)
	return nil
}

func (s *Store) LoadFromFileEncrypted(path string, key []byte) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var enc encryptedPersistedState
	if err := json.Unmarshal(b, &enc); err != nil {
		return fmt.Errorf("parse encrypted auth store envelope: %w", err)
	}
	if strings.TrimSpace(enc.Version) == "" || strings.TrimSpace(enc.Nonce) == "" || strings.TrimSpace(enc.Ciphertext) == "" {
		return fmt.Errorf("encrypted auth store missing required envelope fields")
	}
	rawState, err := decryptState(enc, key)
	if err != nil {
		return err
	}
	var state persistedState
	if err := json.Unmarshal(rawState, &state); err != nil {
		return fmt.Errorf("parse encrypted auth store payload: %w", err)
	}
	s.restoreState(state)
	return nil
}
