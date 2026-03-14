package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
		var rec auditRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return "", count, fmt.Errorf("line %d: invalid json: %w", count+1, err)
		}
		if rec.PrevHash != prev {
			return "", count, fmt.Errorf("line %d: prev hash mismatch", count+1)
		}
		tmp := rec
		tmp.Hash = ""
		b, err := json.Marshal(tmp)
		if err != nil {
			return "", count, fmt.Errorf("line %d: marshal: %w", count+1, err)
		}
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
