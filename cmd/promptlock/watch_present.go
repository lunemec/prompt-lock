package main

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	watchDetailValueLimit  = 96
	watchSummaryValueLimit = 56
	watchIDShortLimit      = 24
	watchSecretsPreviewMax = 2
)

func sanitizeWatchText(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteString(fmt.Sprintf(`\x%02x`, r))
				continue
			}
			if !unicode.IsPrint(r) {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func watchTrimAndSanitize(value string) string {
	return sanitizeWatchText(strings.TrimSpace(value))
}

func watchJoinSanitized(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, watchTrimAndSanitize(value))
	}
	return strings.Join(out, ", ")
}

func watchTruncate(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func watchShortID(value string) string {
	safe := watchTrimAndSanitize(value)
	runes := []rune(safe)
	if len(runes) <= watchIDShortLimit {
		return safe
	}
	return string(runes[:14]) + "..." + string(runes[len(runes)-8:])
}

func watchDisplayValue(value string) string {
	return watchTruncate(watchTrimAndSanitize(value), watchDetailValueLimit)
}

func watchSummaryValue(value string) string {
	return watchTruncate(watchTrimAndSanitize(value), watchSummaryValueLimit)
}

func watchQueueSummary(item pendingItem) string {
	summary := []string{
		watchShortID(item.ID),
		fmt.Sprintf("agent=%s", watchSummaryValue(item.AgentID)),
		fmt.Sprintf("task=%s", watchSummaryValue(item.TaskID)),
	}
	summary = append(summary, watchRequestContextParts(item)...)
	return strings.Join(summary, " | ")
}

func watchSecretsSummary(values []string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, min(len(values), watchSecretsPreviewMax))
	total := 0
	for _, value := range values {
		safe := watchSummaryValue(value)
		if safe == "" {
			continue
		}
		total++
		if len(parts) < watchSecretsPreviewMax {
			parts = append(parts, safe)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	summary := strings.Join(parts, ", ")
	if total > len(parts) {
		summary += fmt.Sprintf(", +%d more", total-len(parts))
	}
	return summary
}

func watchDecisionSummary(item pendingItem) string {
	parts := make([]string, 0, 3)
	if task := watchSummaryValue(item.TaskID); task != "" {
		parts = append(parts, "task="+task)
	}
	parts = append(parts, watchRequestContextParts(item)...)
	if secrets := watchSecretsSummary(item.Secrets); secrets != "" {
		parts = append(parts, "secrets="+secrets)
	}
	if len(parts) == 0 {
		if agent := watchSummaryValue(item.AgentID); agent != "" {
			parts = append(parts, "agent="+agent)
		}
	}
	if len(parts) == 0 {
		return "pending request"
	}
	return strings.Join(parts, " | ")
}

func watchRequestContextParts(item pendingItem) []string {
	parts := make([]string, 0, 4)
	if command := watchSummaryValue(item.CommandSummary); command != "" {
		parts = append(parts, "command="+command)
	}
	if workdir := watchSummaryValue(item.WorkdirSummary); workdir != "" {
		parts = append(parts, "workdir="+workdir)
	}
	if envPath := watchSummaryValue(item.EnvPath); envPath != "" {
		parts = append(parts, "env_path="+envPath)
	}
	if reason := watchSummaryValue(item.Reason); reason != "" {
		parts = append(parts, "reason="+reason)
	}
	return parts
}

func watchActionPastTense(action watchAction) string {
	switch action {
	case watchActionApprove:
		return "approved"
	case watchActionDeny:
		return "denied"
	default:
		return string(action)
	}
}

func watchActionPendingMessage(action watchAction, item pendingItem) string {
	summary := watchDecisionSummary(item)
	if summary == "" {
		return string(action) + " pending"
	}
	return string(action) + " pending: " + summary
}

func watchActionStatusMessage(action watchAction, item pendingItem) string {
	summary := watchDecisionSummary(item)
	if summary == "" {
		return watchActionPastTense(action)
	}
	return watchActionPastTense(action) + ": " + summary
}
