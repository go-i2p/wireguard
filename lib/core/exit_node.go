// Package core provides exit node functionality for routing mesh traffic to the internet.
package core

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/go-i2p/wireguard/lib/metrics"
)

// ExitNode manages the exit node functionality, including IP forwarding,
// NAT/masquerading, policy routing, and traffic management for mesh clients.
//
// CROSS-PLATFORM APPROACH:
// Instead of using OS-specific commands (iptables/sysctl), this implementation uses:
// 1. Platform-specific NAT managers that implement a common interface
// 2. Platform-specific forwarding managers for IP forwarding control
// 3. Platform-specific policy routing managers for upstream VPN routing
// 4. Native OS APIs for NAT and routing configuration
//
// Exit nodes require elevated privileges (CAP_NET_ADMIN or root) to:
//   - Enable IP forwarding via platform-specific APIs
//   - Configure NAT/masquerading via platform firewall
//   - Manage routing tables and policy rules
type ExitNode struct {
	config              ExitNodeConfig
	publicInterface     string
	upstreamInterface   string // Currently active upstream VPN interface
	isActive            bool
	policyRoutingActive bool
	mu                  sync.RWMutex
	natManager          NATManager
	fwdManager          ForwardingManager
	policyManager       PolicyRoutingManager
	metricsCollector    *metrics.ExitMetricsCollector
	metricsUpdateDone   chan struct{} // signal to stop metrics update loop
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

// PolicyRoutingManager defines the cross-platform interface for managing policy routing.
// This enables routing mesh traffic through upstream VPNs for enhanced privacy.
// Platform-specific implementations handle routing tables, policy rules, and interface routing.
type PolicyRoutingManager interface {
	// Setup configures policy routing to route mesh traffic through the upstream VPN interface
	Setup(upstreamInterface, meshInterface string) error
	// Teardown removes all policy routing rules and custom routing tables
	Teardown() error
	// IsActive returns true if policy routing is currently configured
	IsActive() bool
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

	// Create platform-specific policy routing manager
	policyManager, err := newPlatformPolicyRoutingManager()
	if err != nil {
		return nil, fmt.Errorf("create policy routing manager: %w", err)
	}

	return &ExitNode{
		config:              config,
		publicInterface:     config.PublicInterface,
		upstreamInterface:   config.UpstreamVPN,
		isActive:            false,
		policyRoutingActive: false,
		natManager:          natManager,
		fwdManager:          fwdManager,
		policyManager:       policyManager,
		metricsCollector:    metrics.NewExitMetricsCollector(10 * time.Second),
		metricsUpdateDone:   make(chan struct{}),
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

	// Start metrics update loop in background
	go e.updateMetricsLoop()

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

	// Stop metrics update loop
	close(e.metricsUpdateDone)

	// Teardown policy routing first if active
	if e.policyRoutingActive {
		if err := e.policyManager.Teardown(); err != nil {
			log.Warn("failed to teardown policy routing", "error", err)
		}
		e.policyRoutingActive = false
		e.upstreamInterface = ""
	}

	// Remove NAT rules
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

// newPlatformPolicyRoutingManager creates a platform-specific policy routing manager.
func newPlatformPolicyRoutingManager() (PolicyRoutingManager, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxPolicyRoutingManager()
	case "darwin":
		return newDarwinPolicyRoutingManager()
	case "windows":
		return newWindowsPolicyRoutingManager()
	case "freebsd", "openbsd", "netbsd":
		return newBSDPolicyRoutingManager()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// SetupPolicyRouting configures routing to send mesh traffic through an upstream VPN.
// This enables "double VPN" functionality where mesh traffic is routed through both
// the mesh encryption and the upstream VPN for enhanced privacy.
//
// If upstreamInterface is empty and AutoDetect is enabled, this method will
// automatically detect available VPN interfaces and use the first active one.
//
// The mesh interface is assumed to be "wg0" - this may need to be configurable
// in the future for systems with multiple WireGuard interfaces.
func (e *ExitNode) SetupPolicyRouting(upstreamInterface string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.policyRoutingActive {
		return fmt.Errorf("policy routing is already active")
	}

	// Auto-detect upstream VPN if requested and no specific interface provided
	if upstreamInterface == "" && e.config.AutoDetect {
		vpns, err := DetectUpstreamVPNs()
		if err != nil {
			return fmt.Errorf("auto-detect upstream VPNs: %w", err)
		}

		// Find the first active VPN interface
		for _, vpn := range vpns {
			if vpn.IsActive {
				upstreamInterface = vpn.Interface
				log.Info("auto-detected upstream VPN", "interface", upstreamInterface, "type", vpn.Type)
				break
			}
		}

		if upstreamInterface == "" {
			if e.config.FallbackBehavior == "block" {
				return fmt.Errorf("no active upstream VPN found and fallback is set to block")
			}
			log.Info("no upstream VPN detected, routing directly")
			return nil
		}
	}

	// Verify the upstream interface exists and is active
	if upstreamInterface != "" {
		exists, err := InterfaceExists(upstreamInterface)
		if err != nil {
			return fmt.Errorf("check upstream interface %s: %w", upstreamInterface, err)
		}
		if !exists {
			if e.config.FallbackBehavior == "block" {
				return fmt.Errorf("upstream interface %s does not exist and fallback is set to block", upstreamInterface)
			}
			log.Warn("upstream interface does not exist, routing directly", "interface", upstreamInterface)
			return nil
		}
	}

	// Setup policy routing through the upstream interface
	meshInterface := "wg0" // TODO: Make this configurable
	if err := e.policyManager.Setup(upstreamInterface, meshInterface); err != nil {
		return fmt.Errorf("setup policy routing: %w", err)
	}

	e.upstreamInterface = upstreamInterface
	e.policyRoutingActive = true

	log.Info("policy routing configured", "upstream", upstreamInterface, "mesh", meshInterface)
	return nil
}

// TeardownPolicyRouting removes all policy routing configuration and returns
// to direct routing through the public interface.
func (e *ExitNode) TeardownPolicyRouting() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.policyRoutingActive {
		return nil // Already torn down
	}

	if err := e.policyManager.Teardown(); err != nil {
		log.Warn("failed to teardown policy routing", "error", err)
		// Continue with cleanup even if teardown fails
	}

	e.policyRoutingActive = false
	e.upstreamInterface = ""

	log.Info("policy routing torn down")
	return nil
}

// IsPolicyRoutingActive returns whether policy routing is currently configured.
func (e *ExitNode) IsPolicyRoutingActive() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policyRoutingActive
}

// GetUpstreamInterface returns the currently configured upstream VPN interface.
// Returns empty string if no upstream VPN is configured.
func (e *ExitNode) GetUpstreamInterface() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.upstreamInterface
}

// GetMetrics returns the current metrics snapshot.
// The returned metrics are safe to read and modify without affecting the collector.
func (e *ExitNode) GetMetrics() metrics.ExitMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.metricsCollector == nil {
		return metrics.ExitMetrics{}
	}

	return e.metricsCollector.GetMetrics()
}

// GetMetricsCollector returns the metrics collector for direct access.
// This allows external code to record traffic events (bytes sent/received, latency, etc.)
func (e *ExitNode) GetMetricsCollector() *metrics.ExitMetricsCollector {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.metricsCollector
}

// updateMetricsLoop periodically updates bandwidth calculations.
// Runs in a background goroutine until metricsUpdateDone is closed.
func (e *ExitNode) updateMetricsLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.mu.RLock()
			collector := e.metricsCollector
			e.mu.RUnlock()

			if collector != nil {
				collector.CalculateBandwidth()
			}
		case <-e.metricsUpdateDone:
			return
		}
	}
}
