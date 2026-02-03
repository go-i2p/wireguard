//go:build linux

package core

import (
	"fmt"
	"os/exec"
	"strings"
)

// linuxNATManager implements NATManager for Linux using iptables.
type linuxNATManager struct {
	rules []string // Track rules for cleanup
}

func newLinuxNATManager() (NATManager, error) {
	// Check if iptables is available
	if _, err := exec.LookPath("iptables"); err != nil {
		return nil, fmt.Errorf("iptables not found: %w", err)
	}

	return &linuxNATManager{
		rules: make([]string, 0),
	}, nil
}

func (l *linuxNATManager) Setup(publicInterface string) error {
	// NAT rule: masquerade traffic going out the public interface
	natRule := fmt.Sprintf("-t nat -A POSTROUTING -o %s -j MASQUERADE", publicInterface)
	if err := l.addRule(natRule); err != nil {
		return fmt.Errorf("add NAT rule: %w", err)
	}

	// Forward rule: accept traffic from WireGuard interface
	fwdRuleIn := "-A FORWARD -i wg0 -j ACCEPT"
	if err := l.addRule(fwdRuleIn); err != nil {
		return fmt.Errorf("add forward rule (in): %w", err)
	}

	// Forward rule: accept established/related traffic back to WireGuard
	fwdRuleOut := "-A FORWARD -o wg0 -m state --state RELATED,ESTABLISHED -j ACCEPT"
	if err := l.addRule(fwdRuleOut); err != nil {
		return fmt.Errorf("add forward rule (out): %w", err)
	}

	log.Info("linux: NAT rules configured", "interface", publicInterface)
	return nil
}

func (l *linuxNATManager) Teardown() error {
	var errs []string

	// Remove rules in reverse order
	for i := len(l.rules) - 1; i >= 0; i-- {
		rule := l.rules[i]
		// Convert -A to -D to delete the rule
		deleteRule := strings.Replace(rule, " -A ", " -D ", 1)
		if err := runIPTables(deleteRule); err != nil {
			errs = append(errs, fmt.Sprintf("remove rule %q: %v", rule, err))
		}
	}

	l.rules = nil

	if len(errs) > 0 {
		return fmt.Errorf("teardown errors: %s", strings.Join(errs, "; "))
	}

	log.Info("linux: NAT rules removed")
	return nil
}

func (l *linuxNATManager) addRule(rule string) error {
	if err := runIPTables(rule); err != nil {
		return err
	}
	l.rules = append(l.rules, rule)
	return nil
}

func runIPTables(rule string) error {
	args := strings.Fields(rule)
	cmd := exec.Command("iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %w (output: %s)", rule, err, string(output))
	}
	return nil
}

// linuxForwardingManager implements ForwardingManager for Linux using sysctl.
type linuxForwardingManager struct {
	originalIPv4 string
	originalIPv6 string
}

func newLinuxForwardingManager() (ForwardingManager, error) {
	// Check if sysctl is available
	if _, err := exec.LookPath("sysctl"); err != nil {
		return nil, fmt.Errorf("sysctl not found: %w", err)
	}

	return &linuxForwardingManager{}, nil
}

func (l *linuxForwardingManager) SaveState() error {
	// Save IPv4 forwarding state
	ipv4, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		return fmt.Errorf("get IPv4 forward state: %w", err)
	}
	l.originalIPv4 = strings.TrimSpace(ipv4)

	// Save IPv6 forwarding state (optional)
	ipv6, err := getSysctl("net.ipv6.conf.all.forwarding")
	if err != nil {
		log.Debug("could not get IPv6 forward state (optional)", "error", err)
	} else {
		l.originalIPv6 = strings.TrimSpace(ipv6)
	}

	log.Debug("linux: saved forwarding state", "ipv4", l.originalIPv4, "ipv6", l.originalIPv6)
	return nil
}

func (l *linuxForwardingManager) Enable() error {
	// Enable IPv4 forwarding
	if err := setSysctl("net.ipv4.ip_forward", "1"); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	// Try to enable IPv6 forwarding (optional)
	if err := setSysctl("net.ipv6.conf.all.forwarding", "1"); err != nil {
		log.Warn("could not enable IPv6 forwarding (optional)", "error", err)
	}

	log.Info("linux: IP forwarding enabled")
	return nil
}

func (l *linuxForwardingManager) Restore() error {
	// Restore IPv4 forwarding
	if l.originalIPv4 != "" {
		if err := setSysctl("net.ipv4.ip_forward", l.originalIPv4); err != nil {
			return fmt.Errorf("restore IPv4 forwarding: %w", err)
		}
	}

	// Restore IPv6 forwarding (optional)
	if l.originalIPv6 != "" {
		if err := setSysctl("net.ipv6.conf.all.forwarding", l.originalIPv6); err != nil {
			log.Warn("could not restore IPv6 forwarding (optional)", "error", err)
		}
	}

	log.Info("linux: IP forwarding restored")
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

// Stubs for other platforms when building on Linux
func newDarwinNATManager() (NATManager, error) {
	return nil, fmt.Errorf("darwin not supported on linux build")
}

func newWindowsNATManager() (NATManager, error) {
	return nil, fmt.Errorf("windows not supported on linux build")
}

func newBSDNATManager() (NATManager, error) {
	return nil, fmt.Errorf("bsd not supported on linux build")
}

func newDarwinForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("darwin not supported on linux build")
}

func newWindowsForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("windows not supported on linux build")
}

func newBSDForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("bsd not supported on linux build")
}
