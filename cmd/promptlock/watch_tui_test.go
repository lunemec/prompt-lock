package main

import (
	"bufio"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRunWatchSessionUsesTUIForInteractiveTTY(t *testing.T) {
	var ranTUI bool
	var ranPlain bool
	client := &stubWatchClient{}

	err := runWatchSession(client, watchOptions{
		BrokerTarget:   "unix:///tmp/promptlock.sock",
		PollInterval:   2 * time.Second,
		DefaultTTL:     5,
		InteractiveTTY: true,
		KeyboardInput:  strings.NewReader(""),
		Input:          bufio.NewReader(strings.NewReader("")),
		RunInteractive: func(watchClient, watchOptions) error {
			ranTUI = true
			return nil
		},
		RunPlain: func(watchClient, watchOptions) error {
			ranPlain = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runWatchSession returned error: %v", err)
	}
	if !ranTUI {
		t.Fatalf("expected interactive TUI path to run")
	}
	if ranPlain {
		t.Fatalf("expected plain path to stay unused")
	}
}

func TestRunWatchSessionUsesPlainLoopForNonTTY(t *testing.T) {
	var ranTUI bool
	var ranPlain bool
	client := &stubWatchClient{}

	err := runWatchSession(client, watchOptions{
		BrokerTarget:   "unix:///tmp/promptlock.sock",
		PollInterval:   2 * time.Second,
		DefaultTTL:     5,
		InteractiveTTY: false,
		KeyboardInput:  strings.NewReader(""),
		Input:          bufio.NewReader(strings.NewReader("")),
		RunInteractive: func(watchClient, watchOptions) error {
			ranTUI = true
			return nil
		},
		RunPlain: func(watchClient, watchOptions) error {
			ranPlain = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runWatchSession returned error: %v", err)
	}
	if ranTUI {
		t.Fatalf("expected TUI path to stay unused")
	}
	if !ranPlain {
		t.Fatalf("expected plain fallback path to run")
	}
}

func TestRenderWatchTUIIncludesHeaderQueueAndStatus(t *testing.T) {
	item := newPendingItem("req-tui-1")
	item.EnvPath = "./demo.env"
	item.EnvPathCanonical = "/workspace/demo.env"
	item.CommandSummary = `go test ./... -run TestWatch`
	item.WorkdirSummary = "/workspace/project"

	rendered := renderWatchTUI(watchTUIView{
		BrokerTarget:  "unix:///tmp/promptlock.sock",
		PollInterval:  2 * time.Second,
		Now:           time.Date(2026, 3, 22, 9, 30, 0, 0, time.UTC),
		PendingCount:  2,
		Queue:         []pendingItem{item, newPendingItem("req-tui-2")},
		Current:       &item,
		SelectedIndex: 0,
		StatusMessage: "approved req-older",
	})

	for _, want := range []string{
		"PromptLock Watch",
		"Broker",
		"Poll",
		"Pending",
		"Current request",
		"Request:",
		"Intent",
		"Reason",
		"Secrets",
		"Command",
		"Workdir",
		"Command FP",
		"Workdir FP",
		"Env Path",
		"Env Path Canonical",
		"Pending queue",
		"command=go test ./... -run TestWatch",
		"Status: approved req-older",
		"Actions:",
		"Other keys:",
		"j/k select",
		"up/down history",
		"y approve",
		"n deny",
		"s skip",
		"q quit",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected TUI render to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestRenderWatchTUIWithoutCurrentRequestOmitsKeybindings(t *testing.T) {
	rendered := renderWatchTUI(watchTUIView{
		BrokerTarget: "unix:///tmp/promptlock.sock",
		PollInterval: 2 * time.Second,
		Now:          time.Date(2026, 3, 22, 9, 45, 0, 0, time.UTC),
		PendingCount: 0,
		Message:      "Watching for pending requests...",
	})

	if !strings.Contains(rendered, "Status: Watching for pending requests...") {
		t.Fatalf("expected empty watch view to retain status line, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "up/down history") || !strings.Contains(rendered, "q quit") {
		t.Fatalf("expected empty watch view to retain base keybindings, got:\n%s", rendered)
	}
}

func TestRenderWatchTUIIncludesRecentActivity(t *testing.T) {
	rendered := renderWatchTUI(watchTUIView{
		BrokerTarget:  "unix:///tmp/promptlock.sock",
		PollInterval:  2 * time.Second,
		Now:           time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC),
		PendingCount:  0,
		Message:       "Watching for pending requests...",
		HistoryOffset: 1,
		History: []watchActivityEntry{
			{At: time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC), Text: "approved req-1"},
			{At: time.Date(2026, 3, 22, 10, 1, 0, 0, time.UTC), Text: "denied req-2"},
			{At: time.Date(2026, 3, 22, 10, 2, 0, 0, time.UTC), Text: "approved req-3"},
		},
	})

	for _, want := range []string{
		"Recent activity",
		"10:00:00 approved req-1",
		"10:01:00 denied req-2",
		"1 newer events not shown",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected history render to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestRenderWatchTUILocksFooterWhileActionIsPending(t *testing.T) {
	item := newPendingItem("req-pending")
	rendered := renderWatchTUI(watchTUIView{
		BrokerTarget:  "unix:///tmp/promptlock.sock",
		PollInterval:  2 * time.Second,
		Now:           time.Date(2026, 3, 22, 9, 30, 0, 0, time.UTC),
		PendingCount:  1,
		Queue:         []pendingItem{item},
		Current:       &item,
		SelectedIndex: 0,
		ActionPending: true,
		StatusMessage: "approve in flight:\x1b[31mblocked\x1b[0m",
	})

	for _, want := range []string{
		"Action in progress:",
		"input locked until broker responds",
		"Status: approve in flight:\\x1b[31mblocked\\x1b[0m",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected locked footer render to contain %q, got:\n%s", want, rendered)
		}
	}
	for _, bad := range []string{"y approve", "n deny", "s skip", "q quit"} {
		if strings.Contains(rendered, bad) {
			t.Fatalf("expected locked footer to hide %q, got:\n%s", bad, rendered)
		}
	}
}

func TestRenderWatchTUIKeepsSelectedQueueItemVisible(t *testing.T) {
	queue := []pendingItem{
		newPendingItem("req-1"),
		newPendingItem("req-2"),
		newPendingItem("req-3"),
		newPendingItem("req-4"),
		newPendingItem("req-5"),
		newPendingItem("req-6"),
	}
	current := queue[5]
	rendered := renderWatchTUI(watchTUIView{
		BrokerTarget:  "unix:///tmp/promptlock.sock",
		PollInterval:  2 * time.Second,
		Now:           time.Date(2026, 3, 22, 9, 30, 0, 0, time.UTC),
		PendingCount:  len(queue),
		Queue:         queue,
		Current:       &current,
		SelectedIndex: 5,
	})

	if !strings.Contains(rendered, "> 6. req-6") {
		t.Fatalf("expected selected queue item to remain visible, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "> 1. req-1") {
		t.Fatalf("expected queue window to shift off the first item when selecting the last item, got:\n%s", rendered)
	}
}

func TestRenderWatchTUISanitizesAgentControlledFields(t *testing.T) {
	item := newPendingItem("req-\x1b]2;owned\x07")
	item.AgentID = "agent-\x1b[31mred\x1b[0m"
	item.TaskID = "task\nnext"
	item.Reason = "run\tthis"
	item.Secrets = []string{"github_token", "bad\rname"}

	rendered := renderWatchTUI(watchTUIView{
		BrokerTarget: "unix:///tmp/promptlock.sock",
		PollInterval: 2 * time.Second,
		Now:          time.Date(2026, 3, 22, 9, 30, 0, 0, time.UTC),
		PendingCount: 1,
		Queue:        []pendingItem{item},
		Current:      &item,
	})

	for _, bad := range []string{"\x1b", "\x07", "\r", "\nnext"} {
		if strings.Contains(rendered, bad) {
			t.Fatalf("expected rendered output to sanitize %q, got:\n%s", bad, rendered)
		}
	}
	for _, want := range []string{`req-\x1b]2;owned\x07`, `agent-\x1b[31mred\x1b[0m`, `task\nnext`, `run\tthis`, `bad\rname`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected sanitized output to retain escaped %q, got:\n%s", want, rendered)
		}
	}
}

func TestExecuteWatchActionApproveAndDenyPreserveCurrentDefaults(t *testing.T) {
	client := &stubWatchClient{}
	item := newPendingItem("req-act-1")

	approveStatus, err := executeWatchAction(client, &item, watchActionApprove, 7)
	if err != nil {
		t.Fatalf("approve action returned error: %v", err)
	}
	if approveStatus != "approved req-act-1" {
		t.Fatalf("approve status = %q, want approved req-act-1", approveStatus)
	}
	if client.approveCalls != 1 {
		t.Fatalf("approve calls = %d, want 1", client.approveCalls)
	}
	if client.denyCalls != 0 {
		t.Fatalf("deny calls = %d, want 0", client.denyCalls)
	}

	denyStatus, err := executeWatchAction(client, &item, watchActionDeny, 7)
	if err != nil {
		t.Fatalf("deny action returned error: %v", err)
	}
	if denyStatus != "denied req-act-1" {
		t.Fatalf("deny status = %q, want denied req-act-1", denyStatus)
	}
	if client.denyCalls != 1 {
		t.Fatalf("deny calls = %d, want 1", client.denyCalls)
	}
}

func TestWatchTUIModelApproveUpdatesStatusAndRefreshes(t *testing.T) {
	item := newPendingItem("req-tui-approve")
	item.WorkdirSummary = "/workspace/project"
	item.EnvPath = "./demo.env"
	item.EnvPathCanonical = "/workspace/project/demo.env"
	client := &stubWatchClient{
		snapshots: [][]pendingItem{{item}},
	}
	model := newWatchTUIModel(client, watchOptions{
		BrokerTarget: "unix:///tmp/promptlock.sock",
		PollInterval: 2 * time.Second,
		DefaultTTL:   9,
		Now: func() time.Time {
			return time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
		},
	})
	model.applyPendingItems([]pendingItem{item})

	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatalf("expected approve keypress to return a command")
	}
	msg := cmd()
	actionMsg, ok := msg.(watchActionResultMsg)
	if !ok {
		t.Fatalf("command msg type = %T, want watchActionResultMsg", msg)
	}
	if actionMsg.Err != nil {
		t.Fatalf("action msg error = %v", actionMsg.Err)
	}
	wantStatus := "approved: task=task-1 | command=git status --short | workdir=/workspace/project | env_path=./demo.env | reason=run tests | secrets=github_token"
	if actionMsg.StatusMessage != wantStatus {
		t.Fatalf("status message = %q, want %q", actionMsg.StatusMessage, wantStatus)
	}
	if client.approveCalls != 1 {
		t.Fatalf("approve calls = %d, want 1", client.approveCalls)
	}

	updatedModel, refreshCmd := updatedModel.(watchTUIModel).Update(actionMsg)
	if refreshCmd == nil {
		t.Fatalf("expected action result to trigger a refresh command")
	}
	rendered := updatedModel.(watchTUIModel).View()
	if !strings.Contains(rendered, "Status: "+wantStatus) {
		t.Fatalf("expected updated view to include approve status, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Recent activity") {
		t.Fatalf("expected updated view to include recent activity, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, wantStatus) {
		t.Fatalf("expected updated view to include approved action history, got:\n%s", rendered)
	}
}

func TestWatchTUIModelIgnoresConflictingKeypressesWhileActionIsPending(t *testing.T) {
	model := newWatchTUIModel(&stubWatchClient{}, watchOptions{
		BrokerTarget: "unix:///tmp/promptlock.sock",
		PollInterval: 2 * time.Second,
		DefaultTTL:   5,
	})
	model.applyPendingItems([]pendingItem{newPendingItem("req-lock-1")})

	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatalf("expected first approve keypress to return a command")
	}
	lockedModel, secondCmd := updatedModel.(watchTUIModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if secondCmd != nil {
		t.Fatalf("expected conflicting keypress to be ignored while action is pending")
	}
	if !lockedModel.(watchTUIModel).actionPending {
		t.Fatalf("expected model to remain locked while waiting for action result")
	}
}

func TestWatchTUIModelSupportsNavigationSkipAndQuit(t *testing.T) {
	item1 := newPendingItem("req-skip-1")
	item1.WorkdirSummary = "/workspace/project"
	item1.EnvPath = "./demo.env"
	item1.EnvPathCanonical = "/workspace/project/demo.env"
	item2 := newPendingItem("req-skip-2")
	item2.WorkdirSummary = "/workspace/project"
	item2.EnvPath = "./demo.env"
	item2.EnvPathCanonical = "/workspace/project/demo.env"
	model := newWatchTUIModel(&stubWatchClient{}, watchOptions{
		BrokerTarget: "unix:///tmp/promptlock.sock",
		PollInterval: 2 * time.Second,
		DefaultTTL:   5,
		Now: func() time.Time {
			return time.Date(2026, 3, 22, 10, 15, 0, 0, time.UTC)
		},
	})
	model.applyPendingItems([]pendingItem{item1, item2})

	navigatedModel, navCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if navCmd != nil {
		t.Fatalf("navigation should not schedule a broker command")
	}
	if !strings.Contains(navigatedModel.(watchTUIModel).View(), "req-skip-2") {
		t.Fatalf("expected navigation to select the second request")
	}

	skippedModel, skipCmd := navigatedModel.(watchTUIModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if skipCmd != nil {
		t.Fatalf("skip should not schedule a broker command")
	}
	rendered := skippedModel.(watchTUIModel).View()
	if !strings.Contains(rendered, "req-skip-1") {
		t.Fatalf("expected skip to advance back to the remaining request, got:\n%s", rendered)
	}
	wantSkip := "skipped: task=task-1 | command=git status --short | workdir=/workspace/project | env_path=./demo.env | reason=run tests | secrets=github_token"
	if !strings.Contains(rendered, "Status: "+wantSkip) {
		t.Fatalf("expected skip status in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, wantSkip) {
		t.Fatalf("expected skip to be recorded in activity history, got:\n%s", rendered)
	}

	quitModel, quitCmd := skippedModel.(watchTUIModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quitCmd == nil {
		t.Fatalf("quit should return a tea command")
	}
	quitMsg := quitCmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Fatalf("expected quit command to resolve to tea.QuitMsg, got %T", quitMsg)
	}
	if !strings.Contains(quitModel.(watchTUIModel).View(), "watch exited; leaving pending requests untouched") {
		t.Fatalf("expected quit status in final view")
	}
}

func TestWatchTUIModelUsesArrowKeysForHistoryScroll(t *testing.T) {
	model := newWatchTUIModel(&stubWatchClient{}, watchOptions{
		BrokerTarget: "unix:///tmp/promptlock.sock",
		PollInterval: 2 * time.Second,
		DefaultTTL:   5,
	})
	model.applyPendingItems([]pendingItem{newPendingItem("req-history-1")})
	model.history = []watchActivityEntry{
		{At: time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC), Text: "approved: one"},
		{At: time.Date(2026, 3, 22, 10, 1, 0, 0, time.UTC), Text: "approved: two"},
		{At: time.Date(2026, 3, 22, 10, 2, 0, 0, time.UTC), Text: "approved: three"},
		{At: time.Date(2026, 3, 22, 10, 3, 0, 0, time.UTC), Text: "approved: four"},
		{At: time.Date(2026, 3, 22, 10, 4, 0, 0, time.UTC), Text: "approved: five"},
		{At: time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC), Text: "approved: six"},
		{At: time.Date(2026, 3, 22, 10, 6, 0, 0, time.UTC), Text: "approved: seven"},
	}

	olderModel, olderCmd := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if olderCmd != nil {
		t.Fatalf("history scroll should not schedule a broker command")
	}
	if olderModel.(watchTUIModel).historyOffset != 1 {
		t.Fatalf("expected up arrow to scroll to older history, got offset %d", olderModel.(watchTUIModel).historyOffset)
	}

	newerModel, newerCmd := olderModel.(watchTUIModel).Update(tea.KeyMsg{Type: tea.KeyDown})
	if newerCmd != nil {
		t.Fatalf("history scroll should not schedule a broker command")
	}
	if newerModel.(watchTUIModel).historyOffset != 0 {
		t.Fatalf("expected down arrow to scroll back toward newer history, got offset %d", newerModel.(watchTUIModel).historyOffset)
	}
}
