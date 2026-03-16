package app

import "regexp"

var outputRedactors = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{
		pattern:     regexp.MustCompile(`(?im)(authorization\s*:\s*bearer\s+)([^\s"'` + "`" + `]+)`),
		replacement: `${1}[REDACTED_BEARER_TOKEN]`,
	},
	{
		pattern:     regexp.MustCompile(`(?m)\b((?:[A-Z0-9_]*(?:TOKEN|SECRET|API_KEY|PASSWORD))\s*=\s*)(?:"[^"]*"|'[^']*'|[^\s]+)`),
		replacement: `${1}[REDACTED_ENV_VALUE]`,
	},
	{
		pattern:     regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]+\b`),
		replacement: `[REDACTED_GITHUB_TOKEN]`,
	},
	{
		pattern:     regexp.MustCompile(`\bsk-[A-Za-z0-9._-]+\b`),
		replacement: `[REDACTED_API_TOKEN]`,
	},
}

func redactOutput(in string) string {
	s := in
	for _, redactor := range outputRedactors {
		s = redactor.pattern.ReplaceAllString(s, redactor.replacement)
	}
	return s
}
