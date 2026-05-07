//go:build windows

package core

import (
	"fmt"
	"os/exec"
	"strings"
)

// windowsNATManager implements NATManager for Windows using netsh.
type windowsNATManager struct {
	ruleName    string
	rulesActive bool
}

// windowsPolicyRoutingManager implements policy routing for Windows using netsh and route commands.
type windowsPolicyRoutingManager struct {
	routes          []string // Routes added (for cleanup) - format: "subnet ifIndex"
	isActive        bool     // Whether policy routing is currently configured
	meshInterface   string   // Mesh interface (e.g., "Ethernet 2")
	upstreamIface   string   // Upstream VPN interface
	upstreamIfIndex int      // Interface index for upstream interface
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

// Setup configures policy routing on Windows using netsh.
func (w *windowsPolicyRoutingManager) Setup(upstreamInterface, meshInterface string) error {
	if w.isActive {
		return fmt.Errorf("policy routing is already configured")
	}

	if upstreamInterface == "" {
		return fmt.Errorf("upstream interface cannot be empty")
	}

	if meshInterface == "" {
		return fmt.Errorf("mesh interface cannot be empty")
	}

	w.meshInterface = meshInterface
	w.upstreamIface = upstreamInterface

	log.Info("windows: setting up policy routing", "upstream", upstreamInterface, "mesh", meshInterface)

	// Get the interface index for the upstream interface
	ifIndex, err := w.getInterfaceIndex(upstreamInterface)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}
	w.upstreamIfIndex = ifIndex

	log.Info("windows: found interface index", "interface", upstreamInterface, "index", ifIndex)

	// Add route for mesh subnet (10.42.0.0/16) via upstream interface
	// Using netsh: netsh interface ipv4 add route 10.42.0.0/16 <ifIndex>
	meshSubnet := "10.42.0.0/16"

	cmd := exec.Command("netsh", "interface", "ipv4", "add", "route",
		meshSubnet, fmt.Sprintf("%d", ifIndex))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add route for %s: %w (output: %s)", meshSubnet, err, string(output))
	}

	w.routes = append(w.routes, fmt.Sprintf("%s %d", meshSubnet, ifIndex))
	w.isActive = true

	log.Info("windows: policy routing configured successfully", "routes", len(w.routes))
	return nil
}

// getInterfaceIndex retrieves the interface index for a given network interface name.
func (w *windowsPolicyRoutingManager) getInterfaceIndex(ifaceName string) (int, error) {
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "interfaces")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("query interfaces: %w", err)
	}

	// Parse the output to find the interface index
	// Output format:
	// Idx     Met         MTU          State                Name
	// ---  ----------  ----------  ------------  ---------------------------
	//   1          75  4294967295  connected     Loopback Pseudo-Interface 1
	//  12          25        1500  connected     Ethernet

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Idx") || strings.HasPrefix(line, "---") {
			continue
		}

		// Split by whitespace and look for interface name
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// Interface name is the last field(s) - reconstruct in case it has spaces
		interfaceName := strings.Join(fields[4:], " ")
		if interfaceName == ifaceName {
			// First field is the index
			var idx int
			if _, err := fmt.Sscanf(fields[0], "%d", &idx); err == nil {
				return idx, nil
			}
		}
	}

	return 0, fmt.Errorf("interface %s not found", ifaceName)
}

// Teardown removes policy routing configuration on Windows.
func (w *windowsPolicyRoutingManager) Teardown() error {
	if !w.isActive {
		return nil // Already torn down
	}

	log.Info("windows: tearing down policy routing", "routes", len(w.routes))

	var teardownErrors []string

	// Remove all routes (in reverse order)
	for i := len(w.routes) - 1; i >= 0; i-- {
		routeInfo := w.routes[i]
		// Parse "subnet ifIndex" format
		parts := strings.Fields(routeInfo)
		if len(parts) < 2 {
			log.Warn("windows: invalid route format", "route", routeInfo)
			continue
		}

		subnet := parts[0]
		ifIndex := parts[1]

		cmd := exec.Command("netsh", "interface", "ipv4", "delete", "route", subnet, ifIndex)
		if output, err := cmd.CombinedOutput(); err != nil {
			teardownErrors = append(teardownErrors, fmt.Sprintf("remove route %s: %v", subnet, err))
			log.Warn("windows: failed to remove route", "subnet", subnet, "error", err, "output", string(output))
		}
	}

	// Clear state
	w.routes = nil
	w.isActive = false
	w.meshInterface = ""
	w.upstreamIface = ""
	w.upstreamIfIndex = 0

	if len(teardownErrors) > 0 {
		return fmt.Errorf("policy routing teardown had errors: %s", strings.Join(teardownErrors, "; "))
	}

	log.Info("windows: policy routing torn down successfully")
	return nil
}

// IsActive returns whether policy routing is currently configured.
func (w *windowsPolicyRoutingManager) IsActive() bool {
	return w.isActive
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
