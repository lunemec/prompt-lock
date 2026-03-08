package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// VerifyFile validates hash-chain continuity for an audit jsonl file.
func VerifyFile(path string) (string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	prev := ""
	count := 0
	last := ""
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
		count++
	}
	if err := s.Err(); err != nil {
		return "", count, err
	}
	return last, count, nil
}

func WriteCheckpoint(path string, hash string) error {
	return os.WriteFile(path, []byte(strings.TrimSpace(hash)+"\n"), 0o600)
}

func ReadCheckpoint(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
