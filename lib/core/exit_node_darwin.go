//go:build darwin

package core

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// darwinNATManager implements NATManager for macOS using pfctl.
type darwinNATManager struct {
	rulesFile   string
	rulesActive bool
}

// darwinPolicyRoutingManager implements policy routing for macOS using route command.
type darwinPolicyRoutingManager struct {
	routes   []string // Routes added (for cleanup)
	isActive bool     // Whether policy routing is currently configured
}

func newDarwinNATManager() (NATManager, error) {
	// Check if pfctl is available
	if _, err := exec.LookPath("pfctl"); err != nil {
		return nil, fmt.Errorf("pfctl not found: %w", err)
	}

	return &darwinNATManager{
		rulesFile:   "/tmp/i2plan-nat-rules.conf",
		rulesActive: false,
	}, nil
}

func (d *darwinNATManager) Setup(publicInterface string) error {
	// Create pf NAT rules
	rules := fmt.Sprintf(`# i2plan NAT rules
nat on %s from 10.42.0.0/16 to any -> (%s)
pass from 10.42.0.0/16 to any keep state
`, publicInterface, publicInterface)

	// Write rules to temporary file
	if err := os.WriteFile(d.rulesFile, []byte(rules), 0644); err != nil {
		return fmt.Errorf("write pf rules: %w", err)
	}

	// Load rules with pfctl
	cmd := exec.Command("pfctl", "-f", d.rulesFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pfctl load rules: %w (output: %s)", err, string(output))
	}

	// Enable pf if not already enabled
	cmd = exec.Command("pfctl", "-e")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Ignore error if already enabled
		if !strings.Contains(string(output), "already enabled") {
			log.Warn("pfctl enable", "error", err, "output", string(output))
		}
	}

	d.rulesActive = true
	log.Info("darwin: NAT rules configured", "interface", publicInterface)
	return nil
}

func (d *darwinNATManager) Teardown() error {
	if !d.rulesActive {
		return nil
	}

	// Flush NAT rules
	cmd := exec.Command("pfctl", "-F", "nat")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pfctl flush nat: %w (output: %s)", err, string(output))
	}

	// Remove rules file
	_ = os.Remove(d.rulesFile)

	d.rulesActive = false
	log.Info("darwin: NAT rules removed")
	return nil
}

// darwinForwardingManager implements ForwardingManager for macOS using sysctl.
type darwinForwardingManager struct {
	originalIPv4 string
	originalIPv6 string
}

func newDarwinForwardingManager() (ForwardingManager, error) {
	// Check if sysctl is available
	if _, err := exec.LookPath("sysctl"); err != nil {
		return nil, fmt.Errorf("sysctl not found: %w", err)
	}

	return &darwinForwardingManager{}, nil
}

func (d *darwinForwardingManager) SaveState() error {
	// Save IPv4 forwarding state
	ipv4, err := getSysctl("net.inet.ip.forwarding")
	if err != nil {
		return fmt.Errorf("get IPv4 forward state: %w", err)
	}
	d.originalIPv4 = strings.TrimSpace(ipv4)

	// Save IPv6 forwarding state (optional)
	ipv6, err := getSysctl("net.inet6.ip6.forwarding")
	if err != nil {
		log.Debug("could not get IPv6 forward state (optional)", "error", err)
	} else {
		d.originalIPv6 = strings.TrimSpace(ipv6)
	}

	log.Debug("darwin: saved forwarding state", "ipv4", d.originalIPv4, "ipv6", d.originalIPv6)
	return nil
}

func (d *darwinForwardingManager) Enable() error {
	// Enable IPv4 forwarding
	if err := setSysctl("net.inet.ip.forwarding", "1"); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	// Try to enable IPv6 forwarding (optional)
	if err := setSysctl("net.inet6.ip6.forwarding", "1"); err != nil {
		log.Warn("could not enable IPv6 forwarding (optional)", "error", err)
	}

	log.Info("darwin: IP forwarding enabled")
	return nil
}

func (d *darwinForwardingManager) Restore() error {
	// Restore IPv4 forwarding
	if d.originalIPv4 != "" {
		if err := setSysctl("net.inet.ip.forwarding", d.originalIPv4); err != nil {
			return fmt.Errorf("restore IPv4 forwarding: %w", err)
		}
	}

	// Restore IPv6 forwarding (optional)
	if d.originalIPv6 != "" {
		if err := setSysctl("net.inet6.ip6.forwarding", d.originalIPv6); err != nil {
			log.Warn("could not restore IPv6 forwarding (optional)", "error", err)
		}
	}

	log.Info("darwin: IP forwarding restored")
	return nil
}

func setSysctl(key, value string) error {
	cmd := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysctl -w %s=%s: %w (output: %s)", key, value, err, string(output))
	}
	return nil
}

func getSysctl(key string) (string, error) {
	cmd := exec.Command("sysctl", "-n", key)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("sysctl -n %s: %w", key, err)
	}
	return string(output), nil
}

// newDarwinPolicyRoutingManager creates a new macOS policy routing manager.
func newDarwinPolicyRoutingManager() (PolicyRoutingManager, error) {
	// Check if route command is available (should always be present on macOS)
	if _, err := exec.LookPath("route"); err != nil {
		return nil, fmt.Errorf("route command not found: %w", err)
	}

	return &darwinPolicyRoutingManager{
		routes:   make([]string, 0),
		isActive: false,
	}, nil
}

// Setup configures policy routing on macOS (placeholder implementation).
func (d *darwinPolicyRoutingManager) Setup(upstreamInterface, meshInterface string) error {
	if d.isActive {
		return fmt.Errorf("policy routing is already configured")
	}

	if upstreamInterface == "" {
		return fmt.Errorf("upstream interface cannot be empty")
	}

	if meshInterface == "" {
		return fmt.Errorf("mesh interface cannot be empty")
	}

	// TODO: Implement macOS policy routing using route command
	// For now, return not implemented error
	return fmt.Errorf("policy routing not yet implemented on macOS")
}

// Teardown removes policy routing configuration on macOS.
func (d *darwinPolicyRoutingManager) Teardown() error {
	if !d.isActive {
		return nil // Already torn down
	}

	// TODO: Remove routes added during setup
	d.routes = nil
	d.isActive = false
	return nil
}

// IsActive returns whether policy routing is currently configured.
func (d *darwinPolicyRoutingManager) IsActive() bool {
	return d.isActive
}

// Policy routing manager stubs for other platforms
func newLinuxPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("linux not supported on darwin build")
}

func newWindowsPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("windows not supported on darwin build")
}

func newBSDPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("bsd not supported on darwin build")
}

// NAT manager stubs for other platforms
func newLinuxNATManager() (NATManager, error) {
	return nil, fmt.Errorf("linux not supported on darwin build")
}

func newWindowsNATManager() (NATManager, error) {
	return nil, fmt.Errorf("windows not supported on darwin build")
}

func newBSDNATManager() (NATManager, error) {
	return nil, fmt.Errorf("bsd not supported on darwin build")
}

// Forwarding manager stubs for other platforms
func newLinuxForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("linux not supported on darwin build")
}

func newWindowsForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("windows not supported on darwin build")
}

func newBSDForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("bsd not supported on darwin build")
}
