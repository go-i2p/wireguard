// Package core provides exit client functionality for routing traffic through exit nodes.
package core

import (
	"fmt"
	"net/netip"
	"runtime"
	"sync"
)

// ExitClient manages the exit client functionality, routing all traffic through
// a designated exit node in the mesh. It handles route manipulation and
// optional kill switch functionality.
//
// CROSS-PLATFORM APPROACH:
// Instead of using OS-specific commands (iptables/ip), this implementation uses:
// 1. WireGuard's built-in routing via AllowedIPs configuration
// 2. Platform-specific route managers that implement a common interface
// 3. Native OS APIs for route and firewall manipulation
//
// The exit client works by modifying the WireGuard peer configuration to route
// all traffic (0.0.0.0/0) through the exit node peer, rather than manipulating
// the system routing table directly when possible. For advanced use cases that
// require system route manipulation, platform-specific implementations are used.
type ExitClient struct {
	config       ClientExitConfig
	meshIP       netip.Addr // Exit node's mesh IP address
	isActive     bool
	mu           sync.RWMutex
	routeManager RouteManager
	killSwitch   *KillSwitch
}

// RouteManager defines the cross-platform interface for managing routes.
// Platform-specific implementations handle the details of route manipulation.
type RouteManager interface {
	// SaveDefaultRoute saves the current default route for restoration
	SaveDefaultRoute() error
	// AddRoute adds a route to the routing table
	AddRoute(destination netip.Prefix, gateway netip.Addr) error
	// SetDefaultRoute changes the default route
	SetDefaultRoute(gateway netip.Addr) error
	// RestoreRoutes restores all routes to their original state
	RestoreRoutes() error
	// GetDefaultGateway returns the current default gateway
	GetDefaultGateway() (netip.Addr, error)
}

// KillSwitch prevents traffic leaks when the exit node becomes unavailable.
// When enabled, it blocks all non-mesh traffic using platform-specific firewall APIs.
type KillSwitch struct {
	enabled     bool
	wgIface     string
	firewallMgr FirewallManager
	mu          sync.Mutex
}

// FirewallManager defines the cross-platform interface for managing firewall rules.
type FirewallManager interface {
	// BlockNonVPN blocks all traffic except through the VPN interface
	BlockNonVPN(allowedInterface string) error
	// Restore restores firewall to original state
	Restore() error
}

// NewExitClient creates a new exit client instance with the given configuration.
// It validates the configuration but does not activate the exit mode.
// Call Start() to begin routing traffic through the exit node.
func NewExitClient(config ClientExitConfig) (*ExitClient, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("exit client is not enabled in configuration")
	}

	// Create platform-specific route manager
	routeManager, err := newPlatformRouteManager()
	if err != nil {
		return nil, fmt.Errorf("create route manager: %w", err)
	}

	ec := &ExitClient{
		config:       config,
		isActive:     false,
		routeManager: routeManager,
	}

	// Initialize kill switch if enabled
	if config.KillSwitch {
		firewallMgr, err := newPlatformFirewallManager()
		if err != nil {
			return nil, fmt.Errorf("create firewall manager: %w", err)
		}

		ec.killSwitch = &KillSwitch{
			enabled:     true,
			wgIface:     "wg0",
			firewallMgr: firewallMgr,
		}
	}

	return ec, nil
}

// Start activates exit mode by modifying routes to send all traffic through
// the exit node. The exit node must be specified by its mesh IP address.
func (ec *ExitClient) Start(exitNodeMeshIP string) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.isActive {
		return fmt.Errorf("exit client is already active")
	}

	// Parse and validate mesh IP
	meshIP, err := netip.ParseAddr(exitNodeMeshIP)
	if err != nil {
		return fmt.Errorf("invalid exit node mesh IP %q: %w", exitNodeMeshIP, err)
	}
	ec.meshIP = meshIP

	log.Info("starting exit client", "exit_node", exitNodeMeshIP)

	// Save current default route
	if err := ec.routeManager.SaveDefaultRoute(); err != nil {
		return fmt.Errorf("save default route: %w", err)
	}

	// Enable kill switch first if configured (prevents leaks during setup)
	if ec.killSwitch != nil {
		if err := ec.killSwitch.Enable(); err != nil {
			return fmt.Errorf("enable kill switch: %w", err)
		}
	}

	// Setup routing through exit node
	if err := ec.setupRoutes(); err != nil {
		// Rollback kill switch on failure
		if ec.killSwitch != nil {
			_ = ec.killSwitch.Disable()
		}
		return fmt.Errorf("setup routes: %w", err)
	}

	ec.isActive = true
	log.Info("exit client started successfully")
	return nil
}

// Stop deactivates exit mode by restoring original routes and disabling kill switch.
// It is safe to call Stop multiple times.
func (ec *ExitClient) Stop() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.isActive {
		return nil
	}

	log.Info("stopping exit client")

	// Restore routes first
	if err := ec.routeManager.RestoreRoutes(); err != nil {
		log.Warn("failed to restore routes", "error", err)
	}

	// Disable kill switch
	if ec.killSwitch != nil {
		if err := ec.killSwitch.Disable(); err != nil {
			log.Warn("failed to disable kill switch", "error", err)
		}
	}

	ec.isActive = false
	log.Info("exit client stopped")
	return nil
}

// IsActive returns whether the exit client is currently routing through an exit node.
func (ec *ExitClient) IsActive() bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.isActive
}

// setupRoutes configures routing to send all traffic through the exit node.
// It preserves connectivity to the exit node itself via the original gateway.
func (ec *ExitClient) setupRoutes() error {
	// Get original gateway for preserving exit node connectivity
	originalGW, err := ec.routeManager.GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("get default gateway: %w", err)
	}

	// Add specific route to exit node via original gateway (maintains connection)
	exitNodePrefix := netip.PrefixFrom(ec.meshIP, ec.meshIP.BitLen())
	if err := ec.routeManager.AddRoute(exitNodePrefix, originalGW); err != nil {
		return fmt.Errorf("add exit node route: %w", err)
	}

	// Process exclude routes (local network access)
	for _, cidrStr := range ec.config.ExcludeRoutes {
		cidr, err := netip.ParsePrefix(cidrStr)
		if err != nil {
			log.Warn("invalid exclude route", "cidr", cidrStr, "error", err)
			continue
		}
		if err := ec.routeManager.AddRoute(cidr, originalGW); err != nil {
			log.Warn("failed to add exclude route", "cidr", cidrStr, "error", err)
		}
	}

	// Replace default route to go through exit node
	if err := ec.routeManager.SetDefaultRoute(ec.meshIP); err != nil {
		return fmt.Errorf("set default route: %w", err)
	}

	return nil
}

// Enable activates the kill switch by blocking all non-mesh traffic.
func (ks *KillSwitch) Enable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	log.Info("enabling kill switch")

	if err := ks.firewallMgr.BlockNonVPN(ks.wgIface); err != nil {
		return fmt.Errorf("block non-VPN traffic: %w", err)
	}

	log.Info("kill switch enabled")
	return nil
}

// Disable deactivates the kill switch by restoring normal traffic flow.
func (ks *KillSwitch) Disable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	log.Info("disabling kill switch")

	if err := ks.firewallMgr.Restore(); err != nil {
		return fmt.Errorf("restore firewall: %w", err)
	}

	log.Info("kill switch disabled")
	return nil
}

// newPlatformRouteManager creates a platform-specific route manager.
func newPlatformRouteManager() (RouteManager, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxRouteManager()
	case "darwin":
		return newDarwinRouteManager()
	case "windows":
		return newWindowsRouteManager()
	case "freebsd", "openbsd", "netbsd":
		return newBSDRouteManager()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// newPlatformFirewallManager creates a platform-specific firewall manager.
func newPlatformFirewallManager() (FirewallManager, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxFirewallManager()
	case "darwin":
		return newDarwinFirewallManager()
	case "windows":
		return newWindowsFirewallManager()
	case "freebsd", "openbsd", "netbsd":
		return newBSDFirewallManager()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
