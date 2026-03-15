package audit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type storedAuditLine struct {
	EventRaw  json.RawMessage
	PrevHash  string
	Hash      string
	Canonical string
}

// VerifyFile validates hash-chain continuity for an audit jsonl file.
func VerifyFile(path string) (string, int, error) {
	return verifyFile(path, "")
}

// VerifyFileAnchored validates hash-chain continuity and requires the provided
// checkpoint hash to exist somewhere in the verified chain.
func VerifyFileAnchored(path string, checkpointHash string) (string, int, error) {
	return verifyFile(path, strings.TrimSpace(checkpointHash))
}

func verifyFile(path string, checkpointHash string) (string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	prev := ""
	count := 0
	last := ""
	checkpointFound := checkpointHash == ""
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		rec, err := parseStoredAuditLine(line)
		if err != nil {
			return "", count, fmt.Errorf("line %d: %w", count+1, err)
		}
		if rec.PrevHash != prev {
			return "", count, fmt.Errorf("line %d: prev hash mismatch", count+1)
		}
		b := formatStoredAuditLine(rec.EventRaw, rec.PrevHash, "")
		h := sha256.Sum256(b)
		exp := hex.EncodeToString(h[:])
		if rec.Hash != exp {
			return "", count, fmt.Errorf("line %d: hash mismatch", count+1)
		}
		prev = rec.Hash
		last = rec.Hash
		if checkpointHash != "" && rec.Hash == checkpointHash {
			checkpointFound = true
		}
		count++
	}
	if err := s.Err(); err != nil {
		return "", count, err
	}
	if !checkpointFound {
		return "", count, fmt.Errorf("checkpoint hash %s not found in verified audit chain", checkpointHash)
	}
	return last, count, nil
}

func WriteCheckpoint(path string, hash string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".promptlock-checkpoint-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(strings.TrimSpace(hash) + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if err := syncCheckpointParentDir(dir); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func ReadCheckpoint(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

var syncCheckpointParentDir = func(path string) error {
	return syncDir(path)
}

func parseStoredAuditLine(line string) (storedAuditLine, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return storedAuditLine{}, fmt.Errorf("invalid json: empty line")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return storedAuditLine{}, fmt.Errorf("invalid json: %w", err)
	}
	if len(raw) != 3 {
		return storedAuditLine{}, fmt.Errorf("unexpected top-level fields")
	}
	eventRaw, ok := raw["event"]
	if !ok {
		return storedAuditLine{}, fmt.Errorf("missing event field")
	}
	prevRaw, ok := raw["prev_hash"]
	if !ok {
		return storedAuditLine{}, fmt.Errorf("missing prev_hash field")
	}
	hashRaw, ok := raw["hash"]
	if !ok {
		return storedAuditLine{}, fmt.Errorf("missing hash field")
	}
	var prevHash string
	if err := json.Unmarshal(prevRaw, &prevHash); err != nil {
		return storedAuditLine{}, fmt.Errorf("invalid prev_hash: %w", err)
	}
	var hash string
	if err := json.Unmarshal(hashRaw, &hash); err != nil {
		return storedAuditLine{}, fmt.Errorf("invalid hash: %w", err)
	}
	if !json.Valid(eventRaw) {
		return storedAuditLine{}, fmt.Errorf("invalid event payload")
	}
	var compactEvent bytes.Buffer
	if err := json.Compact(&compactEvent, eventRaw); err != nil {
		return storedAuditLine{}, fmt.Errorf("compact event payload: %w", err)
	}
	canonical := string(formatStoredAuditLine(json.RawMessage(compactEvent.Bytes()), prevHash, hash))
	if trimmed != canonical {
		return storedAuditLine{}, fmt.Errorf("non-canonical audit record encoding")
	}
	return storedAuditLine{
		EventRaw:  json.RawMessage(compactEvent.Bytes()),
		PrevHash:  prevHash,
		Hash:      hash,
		Canonical: canonical,
	}, nil
}

func formatStoredAuditLine(eventRaw json.RawMessage, prevHash, hash string) []byte {
	prevQuoted, _ := json.Marshal(prevHash)
	hashQuoted, _ := json.Marshal(hash)
	buf := make([]byte, 0, len(eventRaw)+len(prevQuoted)+len(hashQuoted)+32)
	buf = append(buf, `{"event":`...)
	buf = append(buf, eventRaw...)
	buf = append(buf, `,"prev_hash":`...)
	buf = append(buf, prevQuoted...)
	buf = append(buf, `,"hash":`...)
	buf = append(buf, hashQuoted...)
	buf = append(buf, '}')
	return buf
}
