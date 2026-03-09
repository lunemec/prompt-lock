package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	b, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		fmt.Println("ERROR: CHANGELOG.md missing")
		os.Exit(1)
	}
	if err := validateChangelog(string(b)); err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Println("CHANGELOG validation passed")
}

func validateChangelog(text string) error {
	const unreleased = "## [Unreleased]"
	if !strings.Contains(text, unreleased) {
		return fmt.Errorf(`CHANGELOG.md must contain "%s" section`, unreleased)
	}
	versions, err := parseReleaseVersions(text)
	if err != nil {
		return err
	}
	unrelIdx := strings.Index(text, unreleased)
	otherIdx := len(text) + 1
	for _, v := range versions {
		i := strings.Index(text, "## ["+v+"]")
		if i >= 0 && i < otherIdx {
			otherIdx = i
		}
	}
	if otherIdx < len(text)+1 && unrelIdx > otherIdx {
		return fmt.Errorf("[Unreleased] section must be first among release sections")
	}
	return nil
}

func parseReleaseVersions(text string) ([]string, error) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, 8)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "## [") {
			continue
		}
		end := strings.Index(line, "]")
		if end < 4 {
			continue
		}
		v := line[4:end]
		if v == "Unreleased" {
			continue
		}
		if !isSemver(v) {
			return nil, fmt.Errorf("Invalid release version heading: %s (expected SemVer like 1.2.3)", v)
		}
		out = append(out, v)
	}
	return out, nil
}

func isSemver(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
