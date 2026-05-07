//go:build darwin

package core

import (
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"
)

// darwinRouteManager implements RouteManager for macOS using route(8) command.
type darwinRouteManager struct {
	savedRoutes []savedRoute
	originalGW  netip.Addr
}

type savedRoute struct {
	dest    netip.Prefix
	gateway netip.Addr
}

func newDarwinRouteManager() (RouteManager, error) {
	// Check if route command is available
	if _, err := exec.LookPath("route"); err != nil {
		return nil, fmt.Errorf("route command not found: %w", err)
	}

	return &darwinRouteManager{
		savedRoutes: make([]savedRoute, 0),
	}, nil
}

func (d *darwinRouteManager) SaveDefaultRoute() error {
	gw, err := d.GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("get default gateway: %w", err)
	}
	d.originalGW = gw
	log.Debug("saved default gateway", "gateway", gw)
	return nil
}

func (d *darwinRouteManager) AddRoute(destination netip.Prefix, gateway netip.Addr) error {
	// Use route add command
	cmd := exec.Command("route", "-n", "add", "-net", destination.String(), gateway.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add: %w (output: %s)", err, string(output))
	}

	d.savedRoutes = append(d.savedRoutes, savedRoute{
		dest:    destination,
		gateway: gateway,
	})
	log.Debug("added route", "dest", destination, "gateway", gateway)
	return nil
}

func (d *darwinRouteManager) SetDefaultRoute(gateway netip.Addr) error {
	// Delete existing default route
	cmd := exec.Command("route", "-n", "delete", "default")
	_ = cmd.Run() // Ignore error if no default route exists

	// Add new default route
	cmd = exec.Command("route", "-n", "add", "default", gateway.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add default: %w (output: %s)", err, string(output))
	}

	log.Debug("set default route", "gateway", gateway)
	return nil
}

func (d *darwinRouteManager) RestoreRoutes() error {
	// Remove added routes in reverse order
	for i := len(d.savedRoutes) - 1; i >= 0; i-- {
		route := d.savedRoutes[i]
		cmd := exec.Command("route", "-n", "delete", "-net", route.dest.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Warn("failed to remove route", "dest", route.dest, "error", err, "output", string(output))
		}
	}

	// Restore original default gateway
	if d.originalGW.IsValid() {
		cmd := exec.Command("route", "-n", "delete", "default")
		_ = cmd.Run()

		cmd = exec.Command("route", "-n", "add", "default", d.originalGW.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore default gateway: %w (output: %s)", err, string(output))
		}
		log.Debug("restored default gateway", "gateway", d.originalGW)
	}

	d.savedRoutes = nil
	return nil
}

func (d *darwinRouteManager) GetDefaultGateway() (netip.Addr, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("route get default: %w", err)
	}

	// Parse output: "gateway: 192.168.1.1"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "gateway:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				addr, err := netip.ParseAddr(fields[1])
				if err == nil {
					return addr, nil
				}
			}
		}
	}

	return netip.Addr{}, fmt.Errorf("no default gateway found")
}

// darwinFirewallManager implements FirewallManager for macOS using pf (packet filter).
type darwinFirewallManager struct {
	rulesAdded   bool
	rulesFile    string
	anchorLoaded bool
}

func newDarwinFirewallManager() (FirewallManager, error) {
	// Check if pfctl is available
	if _, err := exec.LookPath("pfctl"); err != nil {
		return nil, fmt.Errorf("pfctl not found: %w", err)
	}

	return &darwinFirewallManager{
		rulesAdded:   false,
		rulesFile:    "/etc/pf.anchors/i2plan-killswitch",
		anchorLoaded: false,
	}, nil
}

func (d *darwinFirewallManager) BlockNonVPN(allowedInterface string) error {
	if d.rulesAdded {
		return fmt.Errorf("firewall rules already active")
	}

	log.Info("darwin: enabling kill switch", "interface", allowedInterface)

	// Create PF rules to block all traffic except through the allowed interface
	rules := fmt.Sprintf(`# i2plan kill switch - block all traffic except VPN
block all
pass on lo0
pass on %s
`, allowedInterface)

	// Write rules to anchor file
	if err := os.WriteFile(d.rulesFile, []byte(rules), 0644); err != nil {
		return fmt.Errorf("write pf rules: %w", err)
	}

	// Load the anchor
	cmd := exec.Command("pfctl", "-a", "i2plan-killswitch", "-f", d.rulesFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Clean up the rules file
		_ = os.Remove(d.rulesFile)
		return fmt.Errorf("load pf anchor: %w (output: %s)", err, string(output))
	}

	// Enable pf if not already enabled
	cmd = exec.Command("pfctl", "-e")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Ignore error if already enabled
		if !strings.Contains(string(output), "already enabled") {
			log.Warn("pfctl enable", "error", err, "output", string(output))
		}
	}

	d.rulesAdded = true
	d.anchorLoaded = true
	log.Info("darwin: kill switch enabled", "interface", allowedInterface)
	return nil
}

func (d *darwinFirewallManager) Restore() error {
	if !d.rulesAdded {
		return nil
	}

	log.Info("darwin: disabling kill switch")

	// Flush the anchor
	if d.anchorLoaded {
		cmd := exec.Command("pfctl", "-a", "i2plan-killswitch", "-F", "all")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Warn("darwin: failed to flush pf anchor", "error", err, "output", string(output))
		}
	}

	// Remove the rules file
	if err := os.Remove(d.rulesFile); err != nil && !os.IsNotExist(err) {
		log.Warn("darwin: failed to remove rules file", "error", err)
	}

	d.rulesAdded = false
	d.anchorLoaded = false
	log.Info("darwin: kill switch disabled")
	return nil
}

// Route manager stubs for other platforms
func newLinuxRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("linux not supported on darwin build")
}

func newWindowsRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("windows not supported on darwin build")
}

func newBSDRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("bsd not supported on darwin build")
}

// Firewall manager stubs for other platforms
func newLinuxFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("linux not supported on darwin build")
}

func newWindowsFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("windows not supported on darwin build")
}

func newBSDFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("bsd not supported on darwin build")
}
