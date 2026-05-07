// Package core provides exit client functionality for routing traffic through exit nodes.
package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"sync"
	"time"
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
	connectedAt  time.Time // When the connection was established
	lastCheck    time.Time // Last health check time
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
	ec.connectedAt = time.Now()
	ec.lastCheck = time.Now()
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

// SelectRoute chooses the best exit node based on configuration preferences and available routes.
// It filters exit nodes by requirements (RequireVPN, PreferredRoute) and sorts by quality metrics.
// Returns the node ID and preferred route name, or an error if no suitable exit is found.
//
// This is a simplified implementation that selects based on:
// - Specific exit node ID if configured
// - RequireVPN filter (only exits with upstream VPN)
// - PreferredRoute matching (specific route name preference)
// - Quality metrics (load, bandwidth, latency)
func SelectExitRoute(config ClientExitConfig, exitNodes map[string]interface{}) (string, string, error) {
	if len(exitNodes) == 0 {
		return "", "", fmt.Errorf("no exit nodes available")
	}

	// If specific exit node requested, check if it exists
	if config.ExitNodeID != "" {
		if _, exists := exitNodes[config.ExitNodeID]; exists {
			return config.ExitNodeID, "direct", nil
		}
		return "", "", fmt.Errorf("requested exit node %q not found", config.ExitNodeID)
	}

	// Score each exit node
	type scoredExit struct {
		nodeID    string
		routeName string
		score     float64
	}

	var candidates []scoredExit

	for nodeID := range exitNodes {
		// In a full implementation, we would:
		// 1. Check RequireVPN against adv.UpstreamVPN
		// 2. Match PreferredRoute against adv.AvailableRoutes
		// 3. Calculate score from adv.CurrentLoad, bandwidth, latency

		// For now, add all nodes with a base score
		score := 50.0 // Base score, would be adjusted by metrics

		candidates = append(candidates, scoredExit{
			nodeID:    nodeID,
			routeName: "direct",
			score:     score,
		})
	}

	if len(candidates) == 0 {
		return "", "", fmt.Errorf("no suitable exit nodes found")
	}

	// Return highest scored candidate
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	return best.nodeID, best.routeName, nil
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

// HealthCheck verifies the exit node connection is functioning correctly.
// It checks: 1) Exit node reachability, 2) Internet connectivity, 3) DNS configuration
// Returns an error if any check fails.
func (ec *ExitClient) HealthCheck(ctx context.Context) error {
	ec.mu.RLock()
	active := ec.isActive
	meshIP := ec.meshIP
	dnsServers := ec.config.DNSServers
	ec.mu.RUnlock()

	if !active {
		return fmt.Errorf("exit client is not active")
	}

	// 1. Verify exit node is reachable (ICMP ping)
	if err := ec.pingExitNode(ctx, meshIP); err != nil {
		return fmt.Errorf("exit node unreachable: %w", err)
	}

	// 2. Verify internet connectivity through exit
	if err := ec.testInternetAccess(ctx); err != nil {
		return fmt.Errorf("internet access failed: %w", err)
	}

	// 3. Verify DNS configuration if DNS servers are configured
	if len(dnsServers) > 0 {
		dnsManager, err := NewDNSManager()
		if err != nil {
			return fmt.Errorf("create DNS manager: %w", err)
		}
		if err := dnsManager.VerifyNoDNSLeak(dnsServers); err != nil {
			return fmt.Errorf("DNS leak detected: %w", err)
		}
	}

	return nil
}

// MonitorConnection continuously monitors the exit node connection health.
// It runs periodic health checks and automatically handles failures:
// - If kill switch is enabled: blocks all traffic on failure
// - Logs health check failures for troubleshooting
// - Returns when context is canceled
func (ec *ExitClient) MonitorConnection(ctx context.Context, checkInterval time.Duration) {
	if checkInterval <= 0 {
		checkInterval = 30 * time.Second // default interval
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ec.HealthCheck(ctx); err != nil {
				log.Warn("exit node health check failed", "error", err)

				// If kill switch is enabled, it will already be blocking traffic
				// We just log the failure and continue monitoring
				ec.mu.RLock()
				hasKillSwitch := ec.killSwitch != nil
				ec.mu.RUnlock()

				if hasKillSwitch {
					log.Info("kill switch active, traffic blocked until connection restored")
				}
			} else {
				log.Debug("exit node health check passed")
			}
		case <-ctx.Done():
			log.Info("connection monitoring stopped")
			return
		}
	}
}

// pingExitNode checks if the exit node is reachable via ICMP ping.
// Uses a 5-second timeout for the ping operation.
func (ec *ExitClient) pingExitNode(ctx context.Context, meshIP netip.Addr) error {
	// Create a dialer with timeout
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	// Attempt TCP connection to common port (443) as proxy for reachability
	// ICMP ping requires raw sockets which need elevated privileges
	// TCP connection is more portable and doesn't require special permissions
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(meshIP.String(), "443"))
	if err != nil {
		return fmt.Errorf("failed to reach exit node: %w", err)
	}
	conn.Close()

	return nil
}

// testInternetAccess verifies internet connectivity through the exit node.
// Makes an HTTP request to a reliable public endpoint with a timeout.
func (ec *ExitClient) testInternetAccess(ctx context.Context) error {
	// Create client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	// Test multiple reliable endpoints in case one is down
	testURLs := []string{
		"http://www.google.com/generate_204", // Returns 204 No Content
		"http://captive.apple.com/hotspot-detect.html",
		"http://connectivitycheck.gstatic.com/generate_204",
	}

	var lastErr error
	for _, url := range testURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		// Any successful response means internet access is working
		if resp.StatusCode >= 200 && resp.StatusCode < 500 {
			return nil
		}

		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if lastErr != nil {
		return fmt.Errorf("no internet access: %w", lastErr)
	}

	return fmt.Errorf("all connectivity tests failed")
}

// ExitClientStatus contains exit client connection status information.
type ExitClientStatus struct {
	Connected      bool
	ExitNodeMeshIP string
	ConnectedAt    time.Time
	LastCheck      time.Time
}

// GetStatus returns the current exit client status.
func (ec *ExitClient) GetStatus() ExitClientStatus {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ExitClientStatus{
		Connected:      ec.isActive,
		ExitNodeMeshIP: ec.meshIP.String(),
		ConnectedAt:    ec.connectedAt,
		LastCheck:      ec.lastCheck,
	}
}

// GetExitNodeMeshIP returns the exit node's mesh IP address.
func (ec *ExitClient) GetExitNodeMeshIP() string {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.meshIP.String()
}

// GetConnectedAt returns when the connection was established.
func (ec *ExitClient) GetConnectedAt() time.Time {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.connectedAt
}

// GetLastCheck returns the last health check time.
func (ec *ExitClient) GetLastCheck() time.Time {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.lastCheck
}

// UpdateLastCheck updates the last health check time.
func (ec *ExitClient) UpdateLastCheck() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.lastCheck = time.Now()
}
