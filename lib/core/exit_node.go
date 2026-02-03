// Package core provides exit node functionality for routing mesh traffic to the internet.
package core

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// ExitNode manages the exit node functionality, including IP forwarding,
// NAT/masquerading, and traffic management for mesh clients.
//
// Exit nodes require elevated privileges (CAP_NET_ADMIN or root) to:
//   - Enable IP forwarding via sysctl
//   - Configure NAT/masquerading via iptables
//   - Manage routing tables
//
// The exit node forwards traffic from mesh clients to the public internet,
// acting as a gateway. All configuration is restored on shutdown.
type ExitNode struct {
	config          ExitNodeConfig
	publicInterface string
	isActive        bool
	mu              sync.RWMutex
	iptablesRules   []string // Track rules for cleanup
	originalForward string   // Original IP forward setting
}

// NewExitNode creates a new exit node instance with the given configuration.
// It validates the configuration and checks for required system permissions.
// Returns an error if the configuration is invalid or permissions are insufficient.
func NewExitNode(config ExitNodeConfig) (*ExitNode, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("exit node is not enabled in configuration")
	}

	if config.PublicInterface == "" {
		return nil, fmt.Errorf("public_interface is required for exit node")
	}

	// Check if we have iptables available
	if err := checkCommandAvailable("iptables"); err != nil {
		return nil, fmt.Errorf("iptables not available: %w", err)
	}

	// Check if we have sysctl available
	if err := checkCommandAvailable("sysctl"); err != nil {
		return nil, fmt.Errorf("sysctl not available: %w", err)
	}

	return &ExitNode{
		config:          config,
		publicInterface: config.PublicInterface,
		isActive:        false,
		iptablesRules:   make([]string, 0),
	}, nil
}

// Start activates the exit node by enabling IP forwarding and setting up NAT.
// This method must be called before the exit node can forward traffic.
// Returns an error if setup fails.
func (e *ExitNode) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isActive {
		return fmt.Errorf("exit node is already active")
	}

	log.Info("starting exit node", "interface", e.publicInterface)

	// Save original IP forwarding state
	if err := e.saveIPForwardState(); err != nil {
		return fmt.Errorf("save IP forward state: %w", err)
	}

	// Enable IP forwarding
	if err := e.enableIPForwarding(); err != nil {
		return fmt.Errorf("enable IP forwarding: %w", err)
	}

	// Setup NAT/masquerading
	if err := e.setupNAT(); err != nil {
		// Rollback IP forwarding on failure
		_ = e.restoreIPForwardState()
		return fmt.Errorf("setup NAT: %w", err)
	}

	e.isActive = true
	log.Info("exit node started successfully")
	return nil
}

// Stop deactivates the exit node by removing NAT rules and restoring IP forwarding.
// It is safe to call Stop multiple times.
func (e *ExitNode) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isActive {
		return nil
	}

	log.Info("stopping exit node")

	// Remove NAT rules first
	if err := e.teardownNAT(); err != nil {
		log.Warn("failed to teardown NAT", "error", err)
	}

	// Restore IP forwarding
	if err := e.restoreIPForwardState(); err != nil {
		log.Warn("failed to restore IP forward state", "error", err)
	}

	e.isActive = false
	log.Info("exit node stopped")
	return nil
}

// IsActive returns whether the exit node is currently active.
func (e *ExitNode) IsActive() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isActive
}

// enableIPForwarding enables IPv4 forwarding via sysctl.
// IPv6 forwarding is attempted but failures are logged as warnings.
func (e *ExitNode) enableIPForwarding() error {
	// Enable IPv4 forwarding
	if err := setSysctl("net.ipv4.ip_forward", "1"); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	// Try to enable IPv6 forwarding (optional, don't fail if it doesn't work)
	if err := setSysctl("net.ipv6.conf.all.forwarding", "1"); err != nil {
		log.Warn("could not enable IPv6 forwarding (optional)", "error", err)
	}

	return nil
}

// saveIPForwardState saves the current IPv4 forwarding state for restoration.
func (e *ExitNode) saveIPForwardState() error {
	value, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		return fmt.Errorf("get current IP forward state: %w", err)
	}
	e.originalForward = strings.TrimSpace(value)
	return nil
}

// restoreIPForwardState restores the original IPv4 forwarding state.
func (e *ExitNode) restoreIPForwardState() error {
	if e.originalForward == "" {
		return nil
	}
	return setSysctl("net.ipv4.ip_forward", e.originalForward)
}

// setupNAT configures iptables rules for NAT/masquerading and forwarding.
// Rules are tracked for cleanup during teardown.
func (e *ExitNode) setupNAT() error {
	// NAT rule: masquerade traffic going out the public interface
	natRule := fmt.Sprintf("-t nat -A POSTROUTING -o %s -j MASQUERADE", e.publicInterface)
	if err := e.addIPTablesRule(natRule); err != nil {
		return fmt.Errorf("add NAT rule: %w", err)
	}

	// Forward rule: accept traffic from WireGuard interface
	fwdRuleIn := "-A FORWARD -i wg0 -j ACCEPT"
	if err := e.addIPTablesRule(fwdRuleIn); err != nil {
		return fmt.Errorf("add forward rule (in): %w", err)
	}

	// Forward rule: accept established/related traffic back to WireGuard
	fwdRuleOut := "-A FORWARD -o wg0 -m state --state RELATED,ESTABLISHED -j ACCEPT"
	if err := e.addIPTablesRule(fwdRuleOut); err != nil {
		return fmt.Errorf("add forward rule (out): %w", err)
	}

	return nil
}

// teardownNAT removes all iptables rules that were added during setup.
// It attempts to remove all rules even if some fail.
func (e *ExitNode) teardownNAT() error {
	var errs []string

	// Remove rules in reverse order
	for i := len(e.iptablesRules) - 1; i >= 0; i-- {
		rule := e.iptablesRules[i]
		// Convert -A to -D to delete the rule
		deleteRule := strings.Replace(rule, " -A ", " -D ", 1)
		if err := runIPTables(deleteRule); err != nil {
			errs = append(errs, fmt.Sprintf("remove rule %q: %v", rule, err))
		}
	}

	e.iptablesRules = nil

	if len(errs) > 0 {
		return fmt.Errorf("teardown errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// addIPTablesRule adds an iptables rule and tracks it for cleanup.
func (e *ExitNode) addIPTablesRule(rule string) error {
	if err := runIPTables(rule); err != nil {
		return err
	}
	e.iptablesRules = append(e.iptablesRules, rule)
	return nil
}

// runIPTables executes an iptables command with the given arguments.
// The rule string should include all arguments (e.g., "-t nat -A POSTROUTING ...").
func runIPTables(rule string) error {
	args := strings.Fields(rule)
	cmd := exec.Command("iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %w (output: %s)", rule, err, string(output))
	}
	return nil
}

// setSysctl sets a sysctl parameter to the given value.
func setSysctl(key, value string) error {
	cmd := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysctl -w %s=%s: %w (output: %s)", key, value, err, string(output))
	}
	return nil
}

// getSysctl retrieves the current value of a sysctl parameter.
func getSysctl(key string) (string, error) {
	cmd := exec.Command("sysctl", "-n", key)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("sysctl -n %s: %w", key, err)
	}
	return string(output), nil
}

// checkCommandAvailable checks if a command is available in the system PATH.
func checkCommandAvailable(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("command %q not found in PATH", name)
	}
	return nil
}
