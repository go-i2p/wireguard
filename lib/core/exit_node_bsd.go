//go:build freebsd || openbsd || netbsd

package core

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// bsdNATManager implements NATManager for BSD systems using pfctl.
type bsdNATManager struct {
	rulesFile   string
	rulesActive bool
}

func newBSDNATManager() (NATManager, error) {
	// Check if pfctl is available
	if _, err := exec.LookPath("pfctl"); err != nil {
		return nil, fmt.Errorf("pfctl not found: %w", err)
	}

	return &bsdNATManager{
		rulesFile:   "/tmp/i2plan-nat-rules.conf",
		rulesActive: false,
	}, nil
}

func (b *bsdNATManager) Setup(publicInterface string) error {
	// Create pf NAT rules
	rules := fmt.Sprintf(`# i2plan NAT rules
nat on %s from 10.42.0.0/16 to any -> (%s)
pass from 10.42.0.0/16 to any keep state
`, publicInterface, publicInterface)

	// Write rules to temporary file
	if err := os.WriteFile(b.rulesFile, []byte(rules), 0644); err != nil {
		return fmt.Errorf("write pf rules: %w", err)
	}

	// Load rules with pfctl
	cmd := exec.Command("pfctl", "-f", b.rulesFile)
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

	b.rulesActive = true
	log.Info("bsd: NAT rules configured", "interface", publicInterface)
	return nil
}

func (b *bsdNATManager) Teardown() error {
	if !b.rulesActive {
		return nil
	}

	// Flush NAT rules
	cmd := exec.Command("pfctl", "-F", "nat")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pfctl flush nat: %w (output: %s)", err, string(output))
	}

	// Remove rules file
	_ = os.Remove(b.rulesFile)

	b.rulesActive = false
	log.Info("bsd: NAT rules removed")
	return nil
}

// bsdForwardingManager implements ForwardingManager for BSD systems using sysctl.
type bsdForwardingManager struct {
	originalIPv4 string
	originalIPv6 string
}

func newBSDForwardingManager() (ForwardingManager, error) {
	// Check if sysctl is available
	if _, err := exec.LookPath("sysctl"); err != nil {
		return nil, fmt.Errorf("sysctl not found: %w", err)
	}

	return &bsdForwardingManager{}, nil
}

func (b *bsdForwardingManager) SaveState() error {
	// Save IPv4 forwarding state
	ipv4, err := getSysctl("net.inet.ip.forwarding")
	if err != nil {
		return fmt.Errorf("get IPv4 forward state: %w", err)
	}
	b.originalIPv4 = strings.TrimSpace(ipv4)

	// Save IPv6 forwarding state (optional)
	ipv6, err := getSysctl("net.inet6.ip6.forwarding")
	if err != nil {
		log.Debug("could not get IPv6 forward state (optional)", "error", err)
	} else {
		b.originalIPv6 = strings.TrimSpace(ipv6)
	}

	log.Debug("bsd: saved forwarding state", "ipv4", b.originalIPv4, "ipv6", b.originalIPv6)
	return nil
}

func (b *bsdForwardingManager) Enable() error {
	// Enable IPv4 forwarding
	if err := setSysctl("net.inet.ip.forwarding", "1"); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	// Try to enable IPv6 forwarding (optional)
	if err := setSysctl("net.inet6.ip6.forwarding", "1"); err != nil {
		log.Warn("could not enable IPv6 forwarding (optional)", "error", err)
	}

	log.Info("bsd: IP forwarding enabled")
	return nil
}

func (b *bsdForwardingManager) Restore() error {
	// Restore IPv4 forwarding
	if b.originalIPv4 != "" {
		if err := setSysctl("net.inet.ip.forwarding", b.originalIPv4); err != nil {
			return fmt.Errorf("restore IPv4 forwarding: %w", err)
		}
	}

	// Restore IPv6 forwarding (optional)
	if b.originalIPv6 != "" {
		if err := setSysctl("net.inet6.ip6.forwarding", b.originalIPv6); err != nil {
			log.Warn("could not restore IPv6 forwarding (optional)", "error", err)
		}
	}

	log.Info("bsd: IP forwarding restored")
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

// newBSDPolicyRoutingManager creates a new BSD policy routing manager.
func newBSDPolicyRoutingManager() (PolicyRoutingManager, error) {
	// Check if route command is available
	if _, err := exec.LookPath("route"); err != nil {
		return nil, fmt.Errorf("route command not found: %w", err)
	}

	return &bsdPolicyRoutingManager{
		routes:   make([]string, 0),
		isActive: false,
	}, nil
}

// Policy routing manager stubs for other platforms
func newLinuxPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("linux not supported on bsd build")
}

func newDarwinPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("darwin not supported on bsd build")
}

func newWindowsPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("windows not supported on bsd build")
}
