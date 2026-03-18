package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type pendingItem struct {
	ID                 string   `json:"ID"`
	AgentID            string   `json:"AgentID"`
	TaskID             string   `json:"TaskID"`
	Intent             string   `json:"Intent"`
	Reason             string   `json:"Reason"`
	TTLMinutes         int      `json:"TTLMinutes"`
	Secrets            []string `json:"Secrets"`
	CommandFingerprint string   `json:"CommandFingerprint"`
	WorkdirFingerprint string   `json:"WorkdirFingerprint"`
	EnvPath            string   `json:"EnvPath"`
	EnvPathCanonical   string   `json:"EnvPathCanonical"`
}

type pendingResponse struct {
	Pending []pendingItem `json:"pending"`
}

const ansiClearScreen = "\x1b[2J\x1b[H"

type watchClient interface {
	ListPending() ([]pendingItem, error)
	Approve(requestID string, ttl int) error
	Deny(requestID, reason string) error
}

type brokerWatchClient struct {
	broker        string
	brokerUnix    string
	operatorToken string
}

func (c brokerWatchClient) ListPending() ([]pendingItem, error) {
	return listPending(c.broker, c.brokerUnix, c.operatorToken)
}

func (c brokerWatchClient) Approve(requestID string, ttl int) error {
	_, err := approve(c.broker, c.brokerUnix, c.operatorToken, requestID, ttl)
	return err
}

func (c brokerWatchClient) Deny(requestID, reason string) error {
	return deny(c.broker, c.brokerUnix, c.operatorToken, requestID, reason)
}

type watchOptions struct {
	BrokerTarget string
	PollInterval time.Duration
	DefaultTTL   int
	Once         bool
	Input        *bufio.Reader
	Output       io.Writer
	ClearScreen  bool
	Now          func() time.Time
	Pause        func(time.Duration)
}

type watchView struct {
	BrokerTarget string
	PollInterval time.Duration
	Now          time.Time
	PendingCount int
	Current      *pendingItem
	MoreQueued   int
	Message      string
}

type watchLoopState struct {
	membershipSignature string
	skipped             map[string]struct{}
	lastRenderKey       string
}

func runWatch(args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			runWatchList(args[1:])
			return
		case "allow":
			runWatchAllow(args[1:])
			return
		case "deny":
			runWatchDeny(args[1:])
			return
		case "help":
			fmt.Print(watchHelpText())
			return
		}
	}
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("watch", flag.ContinueOnError)
		conn := registerBrokerFlags(fs)
		fs.Duration("poll-interval", 3*time.Second, "poll interval")
		fs.Int("ttl", 5, "approval ttl override")
		fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
		fs.Bool("once", false, "process one pass and exit")
		fs.Bool("external", false, "connect to an already-running daemon only (disable auto-start)")
		fs.String("pid-file", getenv("PROMPTLOCK_DAEMON_PID_FILE", ""), "daemon pid file path (auto-start mode; defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
		fs.String("binary", getenv("PROMPTLOCK_DAEMON_BINARY", "promptlockd"), "promptlockd binary path/name for auto-start mode")
		fs.String("config", getenv("PROMPTLOCK_CONFIG", ""), "optional config path for daemon auto-start")
		fs.String("log-file", getenv("PROMPTLOCK_DAEMON_LOG_FILE", ""), "optional daemon log file for auto-start mode (defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
		_ = conn
		printFlagHelp(os.Stdout, watchHelpText(), fs)
		return
	}

	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	poll := fs.Duration("poll-interval", 3*time.Second, "poll interval")
	defaultTTL := fs.Int("ttl", 5, "approval ttl override")
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	once := fs.Bool("once", false, "process one pass and exit")
	external := fs.Bool("external", false, "connect to an already-running daemon only (disable auto-start)")
	pidFile := fs.String("pid-file", getenv("PROMPTLOCK_DAEMON_PID_FILE", ""), "daemon pid file path (auto-start mode; defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
	daemonBinary := fs.String("binary", getenv("PROMPTLOCK_DAEMON_BINARY", "promptlockd"), "promptlockd binary path/name for auto-start mode")
	daemonConfig := fs.String("config", getenv("PROMPTLOCK_CONFIG", ""), "optional config path for daemon auto-start")
	daemonLogFile := fs.String("log-file", getenv("PROMPTLOCK_DAEMON_LOG_FILE", ""), "optional daemon log file for auto-start mode (defaults to config-scoped path when --config/PROMPTLOCK_CONFIG is set)")
	fs.Parse(args)
	if shouldWatchAutostartDaemon(*external, conn) {
		if err := daemonStart(newDaemonFlags(*pidFile, *daemonBinary, *daemonConfig, *daemonLogFile, false)); err != nil {
			fatal(fmt.Errorf("watch auto-start failed: %w", err))
		}
		if err := waitForWatchBrokerReady(conn, *operatorToken, defaultWatchBrokerReadyTimeout); err != nil {
			fatal(fmt.Errorf("watch auto-start failed: %w", err))
		}
	}
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	client := brokerWatchClient{
		broker:        broker.BaseURL,
		brokerUnix:    broker.UnixSocket,
		operatorToken: *operatorToken,
	}
	opts := watchOptions{
		BrokerTarget: watchBrokerTarget(broker.BaseURL, broker.UnixSocket),
		PollInterval: *poll,
		DefaultTTL:   *defaultTTL,
		Once:         *once,
		Input:        bufio.NewReader(os.Stdin),
		Output:       os.Stdout,
		ClearScreen:  isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout),
		Now:          time.Now,
		Pause:        time.Sleep,
	}
	if err := runWatchLoop(client, opts); err != nil {
		fatal(err)
	}
}

func runWatchLoop(client watchClient, opts watchOptions) error {
	if opts.Input == nil {
		opts.Input = bufio.NewReader(os.Stdin)
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Pause == nil {
		opts.Pause = time.Sleep
	}
	state := watchLoopState{skipped: map[string]struct{}{}}

	for {
		items, err := client.ListPending()
		if err != nil {
			return err
		}

		membershipSignature := pendingMembershipSignature(items)
		if membershipSignature != state.membershipSignature {
			state.membershipSignature = membershipSignature
			state.skipped = map[string]struct{}{}
		}

		current, moreQueued, message := selectWatchItem(items, state.skipped)
		renderKey := watchRenderKey(membershipSignature, current, moreQueued, message)
		if renderKey != state.lastRenderKey {
			renderWatchScreen(opts.Output, watchView{
				BrokerTarget: opts.BrokerTarget,
				PollInterval: opts.PollInterval,
				Now:          opts.Now(),
				PendingCount: len(items),
				Current:      current,
				MoreQueued:   moreQueued,
				Message:      message,
			}, opts.ClearScreen)
			state.lastRenderKey = renderKey
		}

		if current == nil {
			if opts.Once {
				return nil
			}
			opts.Pause(opts.PollInterval)
			continue
		}

		decision, err := promptWatchDecision(opts.Input, opts.Output)
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(opts.Output, "\nstdin closed; leaving pending requests untouched")
				return nil
			}
			return err
		}

		switch decision {
		case "approve":
			if err := client.Approve(current.ID, opts.DefaultTTL); err != nil {
				fmt.Fprintln(opts.Output, "approve failed:", err)
			} else {
				fmt.Fprintln(opts.Output, "approved")
			}
			state.lastRenderKey = ""
		case "deny":
			if err := client.Deny(current.ID, "denied by operator"); err != nil {
				fmt.Fprintln(opts.Output, "deny failed:", err)
			} else {
				fmt.Fprintln(opts.Output, "denied")
			}
			state.lastRenderKey = ""
		case "skip":
			state.skipped[current.ID] = struct{}{}
			state.lastRenderKey = ""
		case "quit":
			fmt.Fprintln(opts.Output, "watch exited; leaving pending requests untouched")
			return nil
		}
	}
}

func promptWatchDecision(input *bufio.Reader, out io.Writer) (string, error) {
	for {
		fmt.Fprint(out, "Action? [y]es / [n]o / [s]kip / [q]uit: ")
		line, err := input.ReadString('\n')
		trimmed := strings.ToLower(strings.TrimSpace(line))
		switch trimmed {
		case "y", "yes":
			return "approve", nil
		case "n", "no":
			return "deny", nil
		case "s", "skip":
			return "skip", nil
		case "q", "quit":
			return "quit", nil
		case "":
			if errors.Is(err, io.EOF) {
				return "", io.EOF
			}
			fmt.Fprintln(out, "Enter y, n, s, or q.")
			continue
		default:
			if errors.Is(err, io.EOF) {
				return "", io.EOF
			}
			fmt.Fprintln(out, "Enter y, n, s, or q.")
			continue
		}
	}
}

func runWatchList(args []string) {
	fs := flag.NewFlagSet("watch list", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	fs.Parse(args)
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	items, err := listPending(broker.BaseURL, broker.UnixSocket, *operatorToken)
	if err != nil {
		fatal(err)
	}
	if len(items) == 0 {
		fmt.Println("no pending requests")
		return
	}
	for _, it := range items {
		fmt.Println(formatPendingListLine(it))
	}
}

func runWatchAllow(args []string) {
	fs := flag.NewFlagSet("watch allow", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	ttl := fs.Int("ttl", 5, "approval ttl override")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(errors.New(watchAllowUsage()))
	}
	requestID := fs.Arg(0)
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	if _, err := approve(broker.BaseURL, broker.UnixSocket, *operatorToken, requestID, *ttl); err != nil {
		fatal(err)
	}
	fmt.Println("approved", requestID)
}

func runWatchDeny(args []string) {
	fs := flag.NewFlagSet("watch deny", flag.ExitOnError)
	conn := registerBrokerFlags(fs)
	operatorToken := fs.String("operator-token", getenv("PROMPTLOCK_OPERATOR_TOKEN", ""), "operator token")
	reason := fs.String("reason", "denied by operator", "deny reason")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(errors.New(watchDenyUsage()))
	}
	requestID := fs.Arg(0)
	broker, err := conn.resolve(brokerRoleOperator)
	if err != nil {
		fatal(err)
	}
	if err := deny(broker.BaseURL, broker.UnixSocket, *operatorToken, requestID, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("denied", requestID)
}

func listPending(broker, brokerUnix, operatorToken string) ([]pendingItem, error) {
	resp, err := getAuth(broker, brokerUnix, "/v1/requests/pending", operatorToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, responseError("pending request failed", resp)
	}
	var out pendingResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Pending, nil
}

func selectWatchItem(items []pendingItem, skipped map[string]struct{}) (*pendingItem, int, string) {
	for idx := range items {
		if _, ok := skipped[items[idx].ID]; ok {
			continue
		}
		moreQueued := unskippedPendingCount(items, skipped) - 1
		return &items[idx], moreQueued, ""
	}
	if len(items) == 0 {
		return nil, 0, "Watching for pending requests..."
	}
	return nil, 0, "All current pending requests have been skipped. Watching for queue changes..."
}

func unskippedPendingCount(items []pendingItem, skipped map[string]struct{}) int {
	count := 0
	for _, item := range items {
		if _, ok := skipped[item.ID]; ok {
			continue
		}
		count++
	}
	return count
}

func renderWatchScreen(out io.Writer, view watchView, clearScreen bool) {
	if clearScreen {
		fmt.Fprint(out, ansiClearScreen)
	}
	fmt.Fprintln(out, "PromptLock Watch")
	fmt.Fprintf(out, "Broker: %s | Poll: %s | Time: %s | Pending: %d\n", view.BrokerTarget, view.PollInterval, view.Now.Format(time.RFC3339), view.PendingCount)
	fmt.Fprintln(out)
	if view.Current == nil {
		fmt.Fprintln(out, view.Message)
		if view.PendingCount == 0 {
			fmt.Fprintln(out, "Waiting for the next approval request.")
		}
		return
	}

	it := view.Current
	fmt.Fprintf(out, "Request %s | agent=%s task=%s ttl=%d\n", it.ID, it.AgentID, it.TaskID, it.TTLMinutes)
	if strings.TrimSpace(it.Intent) != "" {
		fmt.Fprintf(out, "Intent: %s\n", it.Intent)
	}
	fmt.Fprintf(out, "Reason: %s\n", it.Reason)
	fmt.Fprintf(out, "Secrets: %s\n", strings.Join(it.Secrets, ", "))
	fmt.Fprintf(out, "Command FP: %s\n", it.CommandFingerprint)
	fmt.Fprintf(out, "Workdir FP: %s\n", it.WorkdirFingerprint)
	if strings.TrimSpace(it.EnvPath) != "" {
		fmt.Fprintf(out, "Env Path: %s\n", it.EnvPath)
	}
	if strings.TrimSpace(it.EnvPathCanonical) != "" {
		fmt.Fprintf(out, "Env Path Canonical: %s\n", it.EnvPathCanonical)
	}
	if view.MoreQueued > 0 {
		fmt.Fprintf(out, "Queued after this: %d\n", view.MoreQueued)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Action: y=yes n=no s=skip q=quit")
}

func watchRenderKey(membershipSignature string, current *pendingItem, moreQueued int, message string) string {
	if current == nil {
		return "waiting|" + membershipSignature + "|" + message
	}
	return fmt.Sprintf("item|%s|%s|%d", membershipSignature, current.ID, moreQueued)
}

func pendingMembershipSignature(items []pendingItem) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, "\x00")
}

func shouldWatchAutostartDaemon(external bool, conn brokerFlags) bool {
	if external {
		return false
	}
	if conn.Broker != nil && strings.TrimSpace(*conn.Broker) != "" {
		return false
	}
	if conn.BrokerUnix != nil && strings.TrimSpace(*conn.BrokerUnix) != "" {
		return false
	}
	if strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_URL")) != "" {
		return false
	}
	if strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_UNIX_SOCKET")) != "" {
		return false
	}
	return true
}

func watchBrokerTarget(broker, brokerUnix string) string {
	if strings.TrimSpace(brokerUnix) != "" {
		return "unix://" + brokerUnix
	}
	return broker
}

func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func formatPendingListLine(it pendingItem) string {
	line := fmt.Sprintf("%s | agent=%s task=%s ttl=%d",
		it.ID,
		it.AgentID,
		it.TaskID,
		it.TTLMinutes,
	)
	if strings.TrimSpace(it.Intent) != "" {
		line += " | intent=" + it.Intent
	}
	line += fmt.Sprintf(" | secrets=%s | reason=%s | command_fp=%s | workdir_fp=%s",
		strings.Join(it.Secrets, ","),
		it.Reason,
		it.CommandFingerprint,
		it.WorkdirFingerprint,
	)
	if strings.TrimSpace(it.EnvPath) != "" {
		line += " | env_path=" + it.EnvPath
	}
	if strings.TrimSpace(it.EnvPathCanonical) != "" {
		line += " | env_path_canonical=" + it.EnvPathCanonical
	}
	return line
}

func deny(broker, brokerUnix, operatorToken, requestID, reason string) error {
	var out map[string]any
	return postJSONAuth(broker, brokerUnix, "/v1/leases/deny?request_id="+requestID, operatorToken, map[string]string{"reason": reason}, &out)
}
