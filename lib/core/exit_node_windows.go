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
