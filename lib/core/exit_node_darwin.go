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
	routes          []string // Routes added (for cleanup)
	isActive        bool     // Whether policy routing is currently configured
	meshInterface   string   // Mesh interface (e.g., "utun0")
	upstreamIface   string   // Upstream VPN interface
	upstreamGateway string   // Gateway for upstream interface
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

// Setup configures policy routing on macOS using route command.
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

	d.meshInterface = meshInterface
	d.upstreamIface = upstreamInterface

	log.Info("darwin: setting up policy routing", "upstream", upstreamInterface, "mesh", meshInterface)

	// Get the gateway for the upstream interface
	gateway, err := d.getInterfaceGateway(upstreamInterface)
	if err != nil {
		return fmt.Errorf("get upstream gateway: %w", err)
	}
	d.upstreamGateway = gateway

	log.Info("darwin: found upstream gateway", "gateway", gateway, "interface", upstreamInterface)

	// Add route for mesh subnet (10.42.0.0/16) via upstream interface
	meshSubnet := "10.42.0.0/16"
	routeCmd := []string{"add", "-net", meshSubnet, "-interface", upstreamInterface}
	if gateway != "" {
		routeCmd = append(routeCmd, "-gateway", gateway)
	}

	cmd := exec.Command("route", routeCmd...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add route for %s: %w (output: %s)", meshSubnet, err, string(output))
	}

	d.routes = append(d.routes, meshSubnet)
	d.isActive = true

	log.Info("darwin: policy routing configured successfully", "routes", len(d.routes))
	return nil
}

// getInterfaceGateway retrieves the gateway address for a given network interface.
func (d *darwinPolicyRoutingManager) getInterfaceGateway(iface string) (string, error) {
	// Use 'route get default' to find the default gateway
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		// Try getting the gateway directly from the interface
		return "", nil // Return empty string to use interface-only routing
	}

	// Parse the output to find the gateway line
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}

	// No gateway found, use interface-only routing
	return "", nil
}

// Teardown removes policy routing configuration on macOS.
func (d *darwinPolicyRoutingManager) Teardown() error {
	if !d.isActive {
		return nil // Already torn down
	}

	log.Info("darwin: tearing down policy routing", "routes", len(d.routes))

	var teardownErrors []string

	// Remove all routes (in reverse order)
	for i := len(d.routes) - 1; i >= 0; i-- {
		subnet := d.routes[i]
		cmd := exec.Command("route", "delete", "-net", subnet)
		if output, err := cmd.CombinedOutput(); err != nil {
			teardownErrors = append(teardownErrors, fmt.Sprintf("remove route %s: %v", subnet, err))
			log.Warn("darwin: failed to remove route", "subnet", subnet, "error", err, "output", string(output))
		}
	}

	// Clear state
	d.routes = nil
	d.isActive = false
	d.meshInterface = ""
	d.upstreamIface = ""
	d.upstreamGateway = ""

	if len(teardownErrors) > 0 {
		return fmt.Errorf("policy routing teardown had errors: %s", strings.Join(teardownErrors, "; "))
	}

	log.Info("darwin: policy routing torn down successfully")
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
