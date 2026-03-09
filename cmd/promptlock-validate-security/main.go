package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var forbiddenPatterns = []string{
	"ghp_",
	"sk-live-",
	"AKIA",
	"-----BEGIN PRIVATE KEY-----",
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
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(clean) == ".pyc" {
			return nil
		}
		if strings.Contains(filepath.ToSlash(clean), "cmd/promptlock-validate-security/") {
			return nil
		}
		b, err := os.ReadFile(clean)
		if err != nil {
			return nil
		}
		for _, token := range forbiddenPatterns {
			if bytes.Contains(b, []byte(token)) {
				out = append(out, violation{path: filepath.ToSlash(clean), pattern: token})
			}
		}
		return nil
	})
	return out, err
}
