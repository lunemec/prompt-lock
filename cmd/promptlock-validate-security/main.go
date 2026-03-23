package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

var forbiddenPatterns = []secretPattern{
	{
		name: "GitHub token",
		re:   regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	},
	{
		name: "GitHub fine-grained token",
		re:   regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	},
	{
		name: "OpenAI live key",
		re:   regexp.MustCompile(`sk-live-[A-Za-z0-9]{10,}`),
	},
	{
		name: "OpenAI API key",
		re:   regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	},
	{
		name: "AWS access key",
		re:   regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		name: "Private key",
		re:   regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	},
	{
		name: "Slack token",
		re:   regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	},
	{
		name: "Bearer token",
		re:   regexp.MustCompile(`Bearer [A-Za-z0-9._-]{20,}`),
	},
}

var allowedSecretLiterals = map[string][]string{
	"internal/adapters/audit/file_test.go": {
		"Bearer sess_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	},
	"internal/app/execution_process_test.go": {
		"Bearer super-secret-bearer-token",
	},
}

// history-only allowlist for a deleted fixture that is still reachable in git history.
// Keep this narrow so reintroducing the file into the working tree still fails the live scan.
var allowedHistoricalSecretBlobs = map[string]string{
	"379db72162a625c831152cdef2840bef840f5e0a": "scripts/validate_security_basics.py",
}

var skippedDirs = map[string]struct{}{
	".git": {},
}

type violation struct {
	path    string
	pattern string
}

func main() {
	root := "."
	violations, err := scan(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Security baseline failed: %v\n", err)
		os.Exit(1)
	}
	if len(violations) > 0 {
		fmt.Println("Security baseline failed: possible secret patterns found:")
		for _, v := range violations {
			fmt.Printf(" - %s contains pattern %q\n", v.path, v.pattern)
		}
		os.Exit(1)
	}
	historyViolations, err := scanGitHistory(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Security baseline failed: %v\n", err)
		os.Exit(1)
	}
	if len(historyViolations) > 0 {
		fmt.Println("Security baseline failed: possible secret patterns found in git history:")
		for _, v := range historyViolations {
			fmt.Printf(" - %s contains pattern %q\n", v.path, v.pattern)
		}
		os.Exit(1)
	}
	fmt.Println("Security baseline checks passed")
}

func scan(root string) ([]violation, error) {
	var out []violation
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		clean := filepath.Clean(path)
		if d.IsDir() {
			if isSkippedRepoLocalDir(root, clean, "dist") ||
				isSkippedRepoLocalDir(root, clean, ".goreleaser-dist") ||
				isSkippedRepoLocalDir(root, clean, ".gocache") ||
				isSkippedRepoLocalDir(root, clean, ".gomodcache") ||
				isSkippedRepoLocalDir(root, clean, ".cache/go-build") ||
				isSkippedRepoLocalDir(root, clean, "cmd/promptlock-validate-security") {
				return filepath.SkipDir
			}
			if _, skip := skippedDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(clean) == ".pyc" {
			return nil
		}
		b, err := os.ReadFile(clean)
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.ToSlash(clean), err)
		}
		rel, err := filepath.Rel(root, clean)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", filepath.ToSlash(clean), err)
		}
		out = append(out, scanBytes(filepath.ToSlash(rel), b)...)
		return nil
	})
	return out, err
}

func scanGitHistory(root string) ([]violation, error) {
	allObjects, err := gitOutput(root, "rev-list", "--objects", "--all")
	if err != nil {
		return nil, fmt.Errorf("list git history objects: %w", err)
	}

	headObjects, err := gitOutput(root, "ls-tree", "-r", "--full-tree", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("list current git tree objects: %w", err)
	}

	type historyBlob struct {
		hash string
		path string
	}

	var blobs []historyBlob
	currentHashes := make(map[string]struct{})
	currentScanner := bufio.NewScanner(strings.NewReader(headObjects))
	currentScanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for currentScanner.Scan() {
		line := currentScanner.Text()
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		fields := strings.Fields(line[:tab])
		if len(fields) < 3 {
			continue
		}
		currentHashes[fields[2]] = struct{}{}
	}
	if err := currentScanner.Err(); err != nil {
		return nil, err
	}

	seenHashes := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(allObjects))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if _, ok := currentHashes[fields[0]]; ok {
			continue
		}
		if allowedPath, ok := allowedHistoricalSecretBlobs[fields[0]]; ok {
			path := filepath.ToSlash(filepath.Clean(strings.Join(fields[1:], " ")))
			if path == allowedPath {
				continue
			}
		}
		if _, ok := seenHashes[fields[0]]; ok {
			continue
		}
		path := filepath.ToSlash(filepath.Clean(strings.Join(fields[1:], " ")))
		if strings.HasPrefix(path, "cmd/promptlock-validate-security/") {
			continue
		}
		seenHashes[fields[0]] = struct{}{}
		blobs = append(blobs, historyBlob{hash: fields[0], path: path})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(blobs) == 0 {
		return nil, nil
	}

	cmd := exec.Command("git", "-C", root, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open git cat-file stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open git cat-file stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start git cat-file: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	reader := bufio.NewReader(stdout)
	var out []violation
	for _, blob := range blobs {
		if _, err := fmt.Fprintln(stdin, blob.hash); err != nil {
			return nil, fmt.Errorf("queue git blob %s (%s): %w", blob.path, blob.hash, err)
		}
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read git blob header for %s (%s): %w", blob.path, blob.hash, err)
		}
		parts := strings.Fields(header)
		if len(parts) < 3 {
			return nil, fmt.Errorf("unexpected git cat-file header for %s (%s): %q", blob.path, blob.hash, strings.TrimSpace(header))
		}
		size, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse git blob size for %s (%s): %w", blob.path, blob.hash, err)
		}
		if size < 0 {
			return nil, fmt.Errorf("invalid git blob size for %s (%s): %d", blob.path, blob.hash, size)
		}
		content := make([]byte, size)
		if _, err := io.ReadFull(reader, content); err != nil {
			return nil, fmt.Errorf("read git blob %s (%s): %w", blob.path, blob.hash, err)
		}
		if _, err := reader.ReadByte(); err != nil {
			return nil, fmt.Errorf("consume git blob terminator for %s (%s): %w", blob.path, blob.hash, err)
		}
		out = append(out, scanBytes(blob.path, content)...)
	}
	return out, nil
}

func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

func isSkippedRepoLocalDir(root, path, want string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return filepath.ToSlash(filepath.Clean(rel)) == want
}

func scanBytes(path string, content []byte) []violation {
	content = scrubAllowedSecretLiterals(path, content)
	var out []violation
	for _, pattern := range forbiddenPatterns {
		if pattern.re.Find(content) != nil {
			out = append(out, violation{path: path, pattern: pattern.name})
		}
	}
	return out
}

func scrubAllowedSecretLiterals(path string, content []byte) []byte {
	allowed, ok := allowedSecretLiterals[path]
	if !ok {
		return content
	}
	scrubbed := append([]byte(nil), content...)
	for _, literal := range allowed {
		scrubbed = bytes.ReplaceAll(scrubbed, []byte(literal), []byte("<promptlock-allowed-fixture>"))
	}
	return scrubbed
}
