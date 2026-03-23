package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const watchHistoryRenderLimit = 6

var (
	watchTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "0", Dark: "15"})
	watchMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "238", Dark: "250"})
	watchPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"}).
			Padding(0, 1)
	watchBodyLabelStyle = lipgloss.NewStyle().
				Bold(true)
	watchStatusStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "18", Dark: "117"})
	watchFooterLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "238", Dark: "250"})
	watchKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "18", Dark: "223"})
	watchSecondaryKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "248"})
	watchApproveKeyStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1).
				Foreground(lipgloss.AdaptiveColor{Light: "15", Dark: "15"}).
				Background(lipgloss.AdaptiveColor{Light: "28", Dark: "41"})
	watchDenyKeyStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1).
				Foreground(lipgloss.AdaptiveColor{Light: "15", Dark: "15"}).
				Background(lipgloss.AdaptiveColor{Light: "160", Dark: "160"})
	watchSkipKeyStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1).
				Foreground(lipgloss.AdaptiveColor{Light: "0", Dark: "0"}).
				Background(lipgloss.AdaptiveColor{Light: "220", Dark: "220"})
)

func renderWatchTUI(view watchTUIView) string {
	header := watchPanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		watchTitleStyle.Render("PromptLock Watch"),
		watchMetaStyle.Render("Broker: "+watchDisplayValue(view.BrokerTarget)),
		watchMetaStyle.Render(fmt.Sprintf("Poll: %s | Time: %s", view.PollInterval, view.Now.Format("2006-01-02 15:04:05 MST"))),
		watchMetaStyle.Render(fmt.Sprintf("Pending: %d", view.PendingCount)),
	))

	bodySections := []string{renderWatchTUIBody(view)}
	if queue := renderWatchTUIQueue(view); queue != "" {
		bodySections = append(bodySections, queue)
	}
	body := watchPanelStyle.Render(strings.Join(bodySections, "\n\n"))

	sections := []string{header, body}
	if history := renderWatchTUIHistory(view); history != "" {
		sections = append(sections, watchPanelStyle.Render(history))
	}

	statusLine := "Status: ready"
	if strings.TrimSpace(view.StatusMessage) != "" {
		statusLine = "Status: " + sanitizeTerminalText(view.StatusMessage)
	} else if strings.TrimSpace(view.Message) != "" {
		statusLine = "Status: " + sanitizeTerminalText(view.Message)
	}

	footerLines := []string{watchStatusStyle.Render(statusLine)}
	if view.ActionPending {
		footerLines = append(footerLines, lipgloss.JoinHorizontal(lipgloss.Left,
			watchFooterLabelStyle.Render("Action in progress:"),
			"  ",
			watchSecondaryKeyStyle.Render("input locked until broker responds"),
		))
	} else if view.Current != nil {
		footerLines = append(footerLines,
			lipgloss.JoinHorizontal(lipgloss.Left,
				watchFooterLabelStyle.Render("Actions:"),
				"  ",
				watchApproveKeyStyle.Render("y approve"),
				"  ",
				watchDenyKeyStyle.Render("n deny"),
				"  ",
				watchSkipKeyStyle.Render("s skip"),
			),
			lipgloss.JoinHorizontal(lipgloss.Left,
				watchFooterLabelStyle.Render("Other keys:"),
				"  ",
				watchSecondaryKeyStyle.Render("j/k select"),
				"  ",
				watchSecondaryKeyStyle.Render("up/down history"),
				"  ",
				watchSecondaryKeyStyle.Render("q quit"),
			),
		)
	} else {
		footerLines = append(footerLines, lipgloss.JoinHorizontal(lipgloss.Left,
			watchFooterLabelStyle.Render("Other keys:"),
			"  ",
			watchSecondaryKeyStyle.Render("up/down history"),
			"  ",
			watchSecondaryKeyStyle.Render("q quit"),
		))
	}

	footer := watchPanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, footerLines...))
	sections = append(sections, footer)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func renderWatchTUIBody(view watchTUIView) string {
	if view.Current == nil {
		lines := []string{view.Message}
		if view.PendingCount == 0 {
			lines = append(lines, "Waiting for the next approval request.")
		}
		return strings.Join(lines, "\n")
	}

	it := view.Current
	lines := []string{
		"Current request",
		renderWatchDetailLine("Request", watchDisplayValue(it.ID)),
		renderWatchDetailLine("Agent", watchDisplayValue(it.AgentID)),
		renderWatchDetailLine("Task", watchDisplayValue(it.TaskID)),
		renderWatchDetailLine("TTL", fmt.Sprintf("%d minutes", it.TTLMinutes)),
	}
	if len(view.Queue) > 0 {
		lines = append(lines, renderWatchDetailLine("Queue", fmt.Sprintf("%d pending | selected %d of %d", len(view.Queue), view.SelectedIndex+1, len(view.Queue))))
	}
	if strings.TrimSpace(it.Intent) != "" {
		lines = append(lines, renderWatchDetailLine("Intent", watchDisplayValue(it.Intent)))
	}
	lines = append(lines,
		renderWatchDetailLine("Reason", watchDisplayValue(it.Reason)),
		renderWatchDetailLine("Secrets", watchJoinSanitized(it.Secrets)),
	)
	if strings.TrimSpace(it.CommandSummary) != "" {
		lines = append(lines, renderWatchDetailLine("Command", watchDisplayValue(it.CommandSummary)))
	}
	if strings.TrimSpace(it.WorkdirSummary) != "" {
		lines = append(lines, renderWatchDetailLine("Workdir", watchDisplayValue(it.WorkdirSummary)))
	}
	lines = append(lines,
		renderWatchDetailLine("Command FP", watchDisplayValue(it.CommandFingerprint)),
		renderWatchDetailLine("Workdir FP", watchDisplayValue(it.WorkdirFingerprint)),
	)
	if strings.TrimSpace(it.EnvPath) != "" {
		lines = append(lines, renderWatchDetailLine("Env Path", watchDisplayValue(it.EnvPath)))
	}
	if strings.TrimSpace(it.EnvPathCanonical) != "" {
		lines = append(lines, renderWatchDetailLine("Env Path Canonical", watchDisplayValue(it.EnvPathCanonical)))
	}
	return strings.Join(lines, "\n")
}

func renderWatchDetailLine(label, value string) string {
	return watchBodyLabelStyle.Render(label+":") + " " + value
}

func renderWatchTUIHistory(view watchTUIView) string {
	if len(view.History) == 0 {
		return ""
	}

	end := len(view.History) - view.HistoryOffset
	if end < 0 {
		end = 0
	}
	start := end - watchHistoryRenderLimit
	if start < 0 {
		start = 0
	}

	lines := []string{watchBodyLabelStyle.Render("Recent activity")}
	if start > 0 {
		lines = append(lines, fmt.Sprintf("%d earlier events not shown", start))
	}
	if end < len(view.History) {
		lines = append(lines, fmt.Sprintf("%d newer events not shown", len(view.History)-end))
	}
	for _, entry := range view.History[start:end] {
		lines = append(lines, fmt.Sprintf("%s %s", entry.At.Format("15:04:05"), watchDisplayValue(entry.Text)))
	}
	return strings.Join(lines, "\n")
}

func renderWatchTUIQueue(view watchTUIView) string {
	if len(view.Queue) <= 1 && view.SkippedCount == 0 {
		return ""
	}

	lines := []string{watchBodyLabelStyle.Render("Pending queue")}
	if len(view.Queue) == 0 {
		lines = append(lines, "No active queue items selected.")
	} else {
		limit := len(view.Queue)
		if limit > 5 {
			limit = 5
		}
		start := 0
		if view.SelectedIndex >= limit {
			start = view.SelectedIndex - limit + 1
		}
		if start+limit > len(view.Queue) {
			start = len(view.Queue) - limit
		}
		for idx, item := range view.Queue[start : start+limit] {
			prefix := " "
			if start+idx == view.SelectedIndex {
				prefix = ">"
			}
			lines = append(lines, fmt.Sprintf("%s %d. %s", prefix, start+idx+1, watchQueueSummary(item)))
		}
		if len(view.Queue) > limit {
			lines = append(lines, fmt.Sprintf("%d more pending items not shown", len(view.Queue)-limit))
		}
	}
	if view.SkippedCount > 0 {
		lines = append(lines, fmt.Sprintf("%d deferred with skip", view.SkippedCount))
	}
	return strings.Join(lines, "\n")
}
