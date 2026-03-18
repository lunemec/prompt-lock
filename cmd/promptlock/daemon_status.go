package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/config"
)

type daemonStatusReport struct {
	Status                  string `json:"status"`
	PID                     int    `json:"pid,omitempty"`
	PIDFile                 string `json:"pid_file,omitempty"`
	ConfigPath              string `json:"config_path,omitempty"`
	AgentTransport          string `json:"agent_transport,omitempty"`
	AgentTransportTarget    string `json:"agent_transport_target,omitempty"`
	AgentTransportReachable bool   `json:"agent_transport_reachable,omitempty"`
	AgentTransportError     string `json:"agent_transport_error,omitempty"`
	AgentBridgeAddress      string `json:"agent_bridge_address,omitempty"`
	AgentBridgeHostURL      string `json:"agent_bridge_host_url,omitempty"`
	AgentBridgeContainerURL string `json:"agent_bridge_container_url,omitempty"`
	AgentBridgeAdvertised   bool   `json:"agent_bridge_advertised,omitempty"`
	AgentBridgeReachable    bool   `json:"agent_bridge_reachable,omitempty"`
	AgentBridgeError        string `json:"agent_bridge_error,omitempty"`
}

func daemonStatus(flags daemonFlags) error {
	report, err := daemonStatusReportFromFlags(flags)
	if err != nil {
		return err
	}
	if flags.JSONOutput {
		writeJSONStdout(report)
		return nil
	}
	printDaemonStatusReport(report)
	return nil
}

func daemonStatusReportFromFlags(flags daemonFlags) (daemonStatusReport, error) {
	report := daemonStatusReport{
		Status:     "stopped",
		PIDFile:    strings.TrimSpace(flags.PIDFile),
		ConfigPath: strings.TrimSpace(flags.Config),
	}

	pid, err := readPIDFile(flags.PIDFile)
	if err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		return daemonStatusReport{}, err
	}
	report.PID = pid
	if !processAlive(pid) {
		report.Status = "stale"
		return report, nil
	}
	report.Status = "running"

	cfg, err := config.Load(flags.Config)
	if err != nil {
		return daemonStatusReport{}, err
	}
	selection := daemonAgentStatusSelection(strings.TrimSpace(flags.Config), cfg)
	report.AgentTransport, report.AgentTransportTarget = describeDaemonAgentTransport(selection)
	report.AgentBridgeAddress = strings.TrimSpace(cfg.AgentBridgeAddress)
	report.AgentBridgeHostURL = daemonBridgeHostURL(report.AgentBridgeAddress)
	report.AgentBridgeContainerURL = dockerBridgeURLForHostAlias(report.AgentBridgeAddress, dockerHostAlias())

	caps, err := brokerCapabilities(selection.BaseURL, selection.UnixSocket)
	if err != nil {
		report.AgentTransportError = err.Error()
	} else {
		report.AgentTransportReachable = true
		if bridgeAddr := strings.TrimSpace(caps.AgentBridgeAddress); bridgeAddr != "" {
			report.AgentBridgeAddress = bridgeAddr
			report.AgentBridgeAdvertised = true
			report.AgentBridgeHostURL = daemonBridgeHostURL(bridgeAddr)
			report.AgentBridgeContainerURL = dockerBridgeURLForHostAlias(bridgeAddr, dockerHostAlias())
		}
	}

	if report.AgentBridgeHostURL != "" {
		if err := probeDaemonBridge(report.AgentBridgeHostURL); err != nil {
			report.AgentBridgeError = err.Error()
		} else {
			report.AgentBridgeReachable = true
		}
	}
	return report, nil
}

func daemonAgentStatusSelection(configPath string, cfg config.Config) brokerSelection {
	if strings.TrimSpace(configPath) == "" {
		if agentSocket := strings.TrimSpace(os.Getenv("PROMPTLOCK_AGENT_UNIX_SOCKET")); agentSocket != "" {
			return brokerSelection{BaseURL: normalizeBrokerURL(cfg.Address), UnixSocket: agentSocket}
		}
		if compatSocket := strings.TrimSpace(os.Getenv("PROMPTLOCK_BROKER_UNIX_SOCKET")); compatSocket != "" {
			return brokerSelection{BaseURL: normalizeBrokerURL(cfg.Address), UnixSocket: compatSocket}
		}
	}
	if agentSocket := strings.TrimSpace(cfg.AgentUnixSocket); agentSocket != "" {
		return brokerSelection{BaseURL: normalizeBrokerURL(cfg.Address), UnixSocket: agentSocket}
	}
	if compatSocket := strings.TrimSpace(cfg.UnixSocket); compatSocket != "" {
		return brokerSelection{BaseURL: normalizeBrokerURL(cfg.Address), UnixSocket: compatSocket}
	}
	return brokerSelection{BaseURL: normalizeBrokerURL(cfg.Address)}
}

func describeDaemonAgentTransport(selection brokerSelection) (string, string) {
	if strings.TrimSpace(selection.UnixSocket) != "" {
		return "unix_socket", selection.UnixSocket
	}
	return "tcp", selection.BaseURL
}

func daemonBridgeHostURL(address string) string {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(trimmed); err == nil && strings.TrimSpace(port) == "0" {
		return ""
	}
	return "http://" + trimmed
}

func probeDaemonBridge(hostURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(hostURL, "/") + "/v1/meta/capabilities")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("bridge probe failed with status %d", resp.StatusCode)
	}
	return nil
}

func printDaemonStatusReport(report daemonStatusReport) {
	switch report.Status {
	case "stopped":
		fmt.Println("promptlockd status: stopped")
		return
	case "stale":
		fmt.Printf("promptlockd status: stale pid file (pid=%d not running)\n", report.PID)
		return
	default:
		fmt.Printf("promptlockd status: running (pid=%d)\n", report.PID)
	}

	if report.AgentTransport != "" {
		if report.AgentTransportReachable {
			fmt.Printf("agent api: reachable via %s %s\n", humanDaemonTransportLabel(report.AgentTransport), report.AgentTransportTarget)
		} else if report.AgentTransportError != "" {
			fmt.Printf("agent api: probe failed via %s %s (%s)\n", humanDaemonTransportLabel(report.AgentTransport), report.AgentTransportTarget, report.AgentTransportError)
		}
	}

	if report.AgentBridgeHostURL == "" && strings.TrimSpace(report.AgentBridgeAddress) == "" {
		fmt.Println("agent bridge: not configured")
		return
	}
	if report.AgentBridgeHostURL == "" {
		fmt.Println("agent bridge: configured with dynamic port; start the daemon and use `promptlock daemon status --json` to discover the active URL")
		return
	}

	if report.AgentBridgeReachable {
		fmt.Printf("agent bridge: reachable on host at %s\n", report.AgentBridgeHostURL)
	} else if report.AgentBridgeError != "" {
		fmt.Printf("agent bridge: configured at %s (probe failed: %s)\n", report.AgentBridgeHostURL, report.AgentBridgeError)
	} else {
		fmt.Printf("agent bridge: configured at %s\n", report.AgentBridgeHostURL)
	}
	if report.AgentBridgeContainerURL != "" {
		fmt.Printf("container bridge url: %s\n", report.AgentBridgeContainerURL)
	}
}

func humanDaemonTransportLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "unix_socket":
		return "unix socket"
	default:
		return "tcp"
	}
}
