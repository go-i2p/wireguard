// Package core provides exit node functionality for routing mesh traffic to the internet.
package core

import (
	"fmt"
	"runtime"
	"sync"
)

// ExitNode manages the exit node functionality, including IP forwarding,
// NAT/masquerading, and traffic management for mesh clients.
//
// CROSS-PLATFORM APPROACH:
// Instead of using OS-specific commands (iptables/sysctl), this implementation uses:
// 1. Platform-specific NAT managers that implement a common interface
// 2. Platform-specific forwarding managers for IP forwarding control
// 3. Native OS APIs for NAT and routing configuration
//
// Exit nodes require elevated privileges (CAP_NET_ADMIN or root) to:
//   - Enable IP forwarding via platform-specific APIs
//   - Configure NAT/masquerading via platform firewall
//   - Manage routing tables
type ExitNode struct {
	config          ExitNodeConfig
	publicInterface string
	isActive        bool
	mu              sync.RWMutex
	natManager      NATManager
	fwdManager      ForwardingManager
}

// NATManager defines the cross-platform interface for managing NAT/masquerading.
// Platform-specific implementations handle the details of firewall configuration.
type NATManager interface {
	// Setup configures NAT/masquerading for the given public interface
	Setup(publicInterface string) error
	// Teardown removes all NAT/masquerading rules
	Teardown() error
}

// ForwardingManager defines the cross-platform interface for managing IP forwarding.
// Platform-specific implementations handle sysctl, registry, or other mechanisms.
type ForwardingManager interface {
	// SaveState saves the current forwarding state for restoration
	SaveState() error
	// Enable enables IP forwarding
	Enable() error
	// Restore restores the original forwarding state
	Restore() error
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

	// Create platform-specific NAT manager
	natManager, err := newPlatformNATManager()
	if err != nil {
		return nil, fmt.Errorf("create NAT manager: %w", err)
	}

	// Create platform-specific forwarding manager
	fwdManager, err := newPlatformForwardingManager()
	if err != nil {
		return nil, fmt.Errorf("create forwarding manager: %w", err)
	}

	return &ExitNode{
		config:          config,
		publicInterface: config.PublicInterface,
		isActive:        false,
		natManager:      natManager,
		fwdManager:      fwdManager,
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
	if err := e.fwdManager.SaveState(); err != nil {
		return fmt.Errorf("save forwarding state: %w", err)
	}

	// Enable IP forwarding
	if err := e.fwdManager.Enable(); err != nil {
		return fmt.Errorf("enable IP forwarding: %w", err)
	}

	// Setup NAT/masquerading
	if err := e.natManager.Setup(e.publicInterface); err != nil {
		// Rollback IP forwarding on failure
		_ = e.fwdManager.Restore()
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
	if err := e.natManager.Teardown(); err != nil {
		log.Warn("failed to teardown NAT", "error", err)
	}

	// Restore IP forwarding
	if err := e.fwdManager.Restore(); err != nil {
		log.Warn("failed to restore forwarding state", "error", err)
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

// newPlatformNATManager creates a platform-specific NAT manager.
func newPlatformNATManager() (NATManager, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxNATManager()
	case "darwin":
		return newDarwinNATManager()
	case "windows":
		return newWindowsNATManager()
	case "freebsd", "openbsd", "netbsd":
		return newBSDNATManager()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// newPlatformForwardingManager creates a platform-specific forwarding manager.
func newPlatformForwardingManager() (ForwardingManager, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxForwardingManager()
	case "darwin":
		return newDarwinForwardingManager()
	case "windows":
		return newWindowsForwardingManager()
	case "freebsd", "openbsd", "netbsd":
		return newBSDForwardingManager()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
