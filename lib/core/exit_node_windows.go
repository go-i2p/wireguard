//go:build windows

package core

import (
	"fmt"
	"os/exec"
)

// windowsNATManager implements NATManager for Windows using netsh.
type windowsNATManager struct {
	ruleName    string
	rulesActive bool
}

func newWindowsNATManager() (NATManager, error) {
	// Check if netsh is available
	if _, err := exec.LookPath("netsh.exe"); err != nil {
		return nil, fmt.Errorf("netsh.exe not found: %w", err)
	}

	return &windowsNATManager{
		ruleName:    "i2plan-nat",
		rulesActive: false,
	}, nil
}

func (w *windowsNATManager) Setup(publicInterface string) error {
	// For MVP, this is a placeholder showing the structure
	// Full implementation would use netsh interface portproxy or Windows NAT features
	log.Info("windows: would configure NAT", "interface", publicInterface)
	w.rulesActive = true
	return nil
}

func (w *windowsNATManager) Teardown() error {
	if !w.rulesActive {
		return nil
	}

	log.Info("windows: would remove NAT rules")
	w.rulesActive = false
	return nil
}

// windowsForwardingManager implements ForwardingManager for Windows using netsh.
type windowsForwardingManager struct {
	originalState string
	interfaceName string
}

func newWindowsForwardingManager() (ForwardingManager, error) {
	// Check if netsh is available
	if _, err := exec.LookPath("netsh.exe"); err != nil {
		return nil, fmt.Errorf("netsh.exe not found: %w", err)
	}

	return &windowsForwardingManager{}, nil
}

func (w *windowsForwardingManager) SaveState() error {
	// Query current IP forwarding state
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "interface")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("query interface state: %w", err)
	}
	w.originalState = string(output)

	log.Debug("windows: saved forwarding state")
	return nil
}

func (w *windowsForwardingManager) Enable() error {
	// For MVP, this is a placeholder
	// Full implementation would use:
	// netsh interface ipv4 set interface "InterfaceName" forwarding=enabled
	log.Info("windows: would enable IP forwarding")
	return nil
}

func (w *windowsForwardingManager) Restore() error {
	// For MVP, this is a placeholder
	log.Info("windows: would restore IP forwarding state")
	return nil
}

// newWindowsPolicyRoutingManager creates a new Windows policy routing manager.
func newWindowsPolicyRoutingManager() (PolicyRoutingManager, error) {
	// Check if route command is available
	if _, err := exec.LookPath("route.exe"); err != nil {
		if _, err := exec.LookPath("route"); err != nil {
			return nil, fmt.Errorf("route command not found: %w", err)
		}
	}

	return &windowsPolicyRoutingManager{
		routes:   make([]string, 0),
		isActive: false,
	}, nil
}

// Policy routing manager stubs for other platforms
func newLinuxPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("linux not supported on windows build")
}

func newDarwinPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("darwin not supported on windows build")
}

func newBSDPolicyRoutingManager() (PolicyRoutingManager, error) {
	return nil, fmt.Errorf("bsd not supported on windows build")
}

// NAT manager stubs for other platforms
func newLinuxNATManager() (NATManager, error) {
	return nil, fmt.Errorf("linux not supported on windows build")
}

func newDarwinNATManager() (NATManager, error) {
	return nil, fmt.Errorf("darwin not supported on windows build")
}

func newBSDNATManager() (NATManager, error) {
	return nil, fmt.Errorf("bsd not supported on windows build")
}

// Forwarding manager stubs for other platforms
func newLinuxForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("linux not supported on windows build")
}

func newDarwinForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("darwin not supported on windows build")
}

func newBSDForwardingManager() (ForwardingManager, error) {
	return nil, fmt.Errorf("bsd not supported on windows build")
}
