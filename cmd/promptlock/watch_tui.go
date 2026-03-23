package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type watchAction string

const (
	watchActionApprove watchAction = "approve"
	watchActionDeny    watchAction = "deny"
)

type watchPendingResultMsg struct {
	Items []pendingItem
	Err   error
}

type watchPollTickMsg struct{}

type watchActionResultMsg struct {
	Action        watchAction
	RequestID     string
	Summary       string
	StatusMessage string
	Err           error
}

type watchActivityEntry struct {
	At   time.Time
	Text string
}

type watchTUIView struct {
	BrokerTarget  string
	PollInterval  time.Duration
	Now           time.Time
	PendingCount  int
	Queue         []pendingItem
	Current       *pendingItem
	SelectedIndex int
	SkippedCount  int
	Message       string
	StatusMessage string
	History       []watchActivityEntry
	HistoryOffset int
	ActionPending bool
}

type watchTUIModel struct {
	client        watchClient
	brokerTarget  string
	pollInterval  time.Duration
	defaultTTL    int
	now           func() time.Time
	items         []pendingItem
	state         watchLoopState
	selectedID    string
	historyOffset int
	statusMessage string
	actionPending bool
	pendingAction string
	history       []watchActivityEntry
	fatalErr      error
}

func runWatchTUI(client watchClient, opts watchOptions) error {
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.KeyboardInput == nil {
		opts.KeyboardInput = os.Stdin
	}

	program := tea.NewProgram(
		newWatchTUIModel(client, opts),
		tea.WithInput(opts.KeyboardInput),
		tea.WithOutput(opts.Output),
	)
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	model, ok := finalModel.(watchTUIModel)
	if !ok {
		return nil
	}
	if model.statusMessage == "watch exited; leaving pending requests untouched" {
		_, _ = io.WriteString(opts.Output, model.statusMessage+"\n")
	}
	return model.fatalErr
}

func newWatchTUIModel(client watchClient, opts watchOptions) watchTUIModel {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return watchTUIModel{
		client:       client,
		brokerTarget: opts.BrokerTarget,
		pollInterval: opts.PollInterval,
		defaultTTL:   opts.DefaultTTL,
		now:          nowFn,
		state: watchLoopState{
			skipped: map[string]struct{}{},
		},
	}
}

func (m watchTUIModel) Init() tea.Cmd {
	return tea.Batch(
		watchFetchPendingCmd(m.client),
		watchPollTickCmd(m.pollInterval),
	)
}

func (m watchTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case watchPendingResultMsg:
		if msg.Err != nil {
			m.fatalErr = msg.Err
			return m, tea.Quit
		}
		m.applyPendingItems(msg.Items)
		return m, nil
	case watchPollTickMsg:
		return m, tea.Batch(
			watchFetchPendingCmd(m.client),
			watchPollTickCmd(m.pollInterval),
		)
	case watchActionResultMsg:
		m.actionPending = false
		m.pendingAction = ""
		if msg.Err != nil {
			subject := msg.RequestID
			if strings.TrimSpace(msg.Summary) != "" {
				subject = msg.Summary
			}
			m.statusMessage = sanitizeTerminalText(fmt.Sprintf("%s failed: %s: %v", msg.Action, subject, msg.Err))
			return m, nil
		}
		m.statusMessage = msg.StatusMessage
		m.appendHistory(msg.StatusMessage)
		return m, watchFetchPendingCmd(m.client)
	case tea.KeyMsg:
		return m.handleKey(msg.String())
	default:
		return m, nil
	}
}

func (m watchTUIModel) View() string {
	queue := visibleWatchItems(m.items, m.state.skipped)
	current, selectedIndex := selectedWatchItem(queue, m.selectedID)
	message := watchQueueMessage(len(m.items), len(queue))
	return renderWatchTUI(watchTUIView{
		BrokerTarget:  m.brokerTarget,
		PollInterval:  m.pollInterval,
		Now:           m.now(),
		PendingCount:  len(m.items),
		Queue:         queue,
		Current:       current,
		SelectedIndex: selectedIndex,
		SkippedCount:  len(m.state.skipped),
		Message:       message,
		StatusMessage: m.statusMessage,
		History:       m.history,
		HistoryOffset: m.historyOffset,
		ActionPending: m.actionPending,
	})
}

func (m *watchTUIModel) applyPendingItems(items []pendingItem) {
	m.items = items
	m.state.membershipSignature = pendingMembershipSignature(items)
	m.state.skipped = pruneWatchSkipped(m.state.skipped, items)
	m.ensureSelection()
}

func (m watchTUIModel) handleKey(key string) (tea.Model, tea.Cmd) {
	if m.actionPending {
		m.statusMessage = m.pendingAction
		return m, nil
	}

	switch key {
	case "ctrl+c", "q":
		m.statusMessage = "watch exited; leaving pending requests untouched"
		return m, tea.Quit
	case "j":
		m.moveSelection(1)
		return m, nil
	case "k":
		m.moveSelection(-1)
		return m, nil
	case "up", "[":
		m.scrollHistory(1)
		return m, nil
	case "down", "]":
		m.scrollHistory(-1)
		return m, nil
	case "s":
		current := m.selectedItem()
		if current == nil {
			return m, nil
		}
		m.state.skipped[current.ID] = struct{}{}
		m.statusMessage = "skipped: " + watchDecisionSummary(*current)
		m.appendHistory(m.statusMessage)
		m.ensureSelection()
		return m, nil
	case "y", "n":
		current := m.selectedItem()
		if current == nil {
			return m, nil
		}
		item := *current
		action := watchActionApprove
		if key == "n" {
			action = watchActionDeny
		}
		m.actionPending = true
		m.pendingAction = watchActionPendingMessage(action, item)
		m.statusMessage = m.pendingAction
		return m, watchActionCmd(m.client, &item, action, m.defaultTTL)
	default:
		return m, nil
	}
}

func (m *watchTUIModel) appendHistory(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	m.history = append(m.history, watchActivityEntry{
		At:   m.now(),
		Text: text,
	})
}

func watchFetchPendingCmd(client watchClient) tea.Cmd {
	return func() tea.Msg {
		items, err := client.ListPending()
		if err != nil {
			return watchPendingResultMsg{Err: err}
		}
		return watchPendingResultMsg{Items: items}
	}
}

func watchPollTickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return watchPollTickMsg{}
	})
}

func watchActionCmd(client watchClient, item *pendingItem, action watchAction, ttl int) tea.Cmd {
	return func() tea.Msg {
		status, err := executeWatchAction(client, item, action, ttl)
		summary := ""
		if item != nil {
			summary = watchDecisionSummary(*item)
			if err == nil {
				status = watchActionStatusMessage(action, *item)
			}
		}
		return watchActionResultMsg{
			Action:        action,
			RequestID:     item.ID,
			Summary:       summary,
			StatusMessage: status,
			Err:           err,
		}
	}
}

func (m *watchTUIModel) selectedItem() *pendingItem {
	queue := visibleWatchItems(m.items, m.state.skipped)
	current, _ := selectedWatchItem(queue, m.selectedID)
	return current
}

func (m *watchTUIModel) ensureSelection() {
	queue := visibleWatchItems(m.items, m.state.skipped)
	if len(queue) == 0 {
		m.selectedID = ""
		return
	}
	for _, item := range queue {
		if item.ID == m.selectedID {
			return
		}
	}
	m.selectedID = queue[0].ID
}

func (m *watchTUIModel) moveSelection(delta int) {
	queue := visibleWatchItems(m.items, m.state.skipped)
	if len(queue) == 0 {
		m.selectedID = ""
		return
	}
	_, idx := selectedWatchItem(queue, m.selectedID)
	next := idx + delta
	if next < 0 {
		next = 0
	}
	if next >= len(queue) {
		next = len(queue) - 1
	}
	m.selectedID = queue[next].ID
}

func (m *watchTUIModel) scrollHistory(delta int) {
	maxOffset := len(m.history) - watchHistoryRenderLimit
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.historyOffset += delta
	if m.historyOffset < 0 {
		m.historyOffset = 0
	}
	if m.historyOffset > maxOffset {
		m.historyOffset = maxOffset
	}
}

func visibleWatchItems(items []pendingItem, skipped map[string]struct{}) []pendingItem {
	queue := make([]pendingItem, 0, len(items))
	for _, item := range items {
		if _, ok := skipped[item.ID]; ok {
			continue
		}
		queue = append(queue, item)
	}
	return queue
}

func selectedWatchItem(items []pendingItem, selectedID string) (*pendingItem, int) {
	if len(items) == 0 {
		return nil, 0
	}
	for idx := range items {
		if items[idx].ID == selectedID {
			return &items[idx], idx
		}
	}
	return &items[0], 0
}

func watchQueueMessage(totalItems, visibleItems int) string {
	switch {
	case totalItems == 0:
		return "Watching for pending requests..."
	case visibleItems == 0:
		return "All current pending requests are deferred with skip. New requests will still appear; skipped requests stay hidden until they leave the queue."
	default:
		return ""
	}
}

func executeWatchAction(client watchClient, item *pendingItem, action watchAction, ttl int) (string, error) {
	if item == nil {
		return "", fmt.Errorf("no pending request selected")
	}

	switch action {
	case watchActionApprove:
		if err := client.Approve(item.ID, ttl); err != nil {
			return "", err
		}
		return "approved " + item.ID, nil
	case watchActionDeny:
		if err := client.Deny(item.ID, "denied by operator"); err != nil {
			return "", err
		}
		return "denied " + item.ID, nil
	default:
		return "", fmt.Errorf("unsupported watch action %q", action)
	}
}
