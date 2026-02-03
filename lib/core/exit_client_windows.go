//go:build windows

package core

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
)

// windowsRouteManager implements RouteManager for Windows using route.exe and netsh.
type windowsRouteManager struct {
	savedRoutes []savedRoute
	originalGW  netip.Addr
}

type savedRoute struct {
	dest    netip.Prefix
	gateway netip.Addr
}

func newWindowsRouteManager() (RouteManager, error) {
	// Check if route command is available
	if _, err := exec.LookPath("route.exe"); err != nil {
		return nil, fmt.Errorf("route.exe not found: %w", err)
	}

	return &windowsRouteManager{
		savedRoutes: make([]savedRoute, 0),
	}, nil
}

func (w *windowsRouteManager) SaveDefaultRoute() error {
	gw, err := w.GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("get default gateway: %w", err)
	}
	w.originalGW = gw
	log.Debug("saved default gateway", "gateway", gw)
	return nil
}

func (w *windowsRouteManager) AddRoute(destination netip.Prefix, gateway netip.Addr) error {
	// Use route add command
	cmd := exec.Command("route", "ADD", destination.String(), gateway.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add: %w (output: %s)", err, string(output))
	}

	w.savedRoutes = append(w.savedRoutes, savedRoute{
		dest:    destination,
		gateway: gateway,
	})
	log.Debug("added route", "dest", destination, "gateway", gateway)
	return nil
}

func (w *windowsRouteManager) SetDefaultRoute(gateway netip.Addr) error {
	// Delete existing default route
	cmd := exec.Command("route", "DELETE", "0.0.0.0")
	_ = cmd.Run() // Ignore error if no default route exists

	// Add new default route
	cmd = exec.Command("route", "ADD", "0.0.0.0", "MASK", "0.0.0.0", gateway.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add default: %w (output: %s)", err, string(output))
	}

	log.Debug("set default route", "gateway", gateway)
	return nil
}

func (w *windowsRouteManager) RestoreRoutes() error {
	// Remove added routes in reverse order
	for i := len(w.savedRoutes) - 1; i >= 0; i-- {
		route := w.savedRoutes[i]
		cmd := exec.Command("route", "DELETE", route.dest.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Warn("failed to remove route", "dest", route.dest, "error", err, "output", string(output))
		}
	}

	// Restore original default gateway
	if w.originalGW.IsValid() {
		cmd := exec.Command("route", "DELETE", "0.0.0.0")
		_ = cmd.Run()

		cmd = exec.Command("route", "ADD", "0.0.0.0", "MASK", "0.0.0.0", w.originalGW.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore default gateway: %w (output: %s)", err, string(output))
		}
		log.Debug("restored default gateway", "gateway", w.originalGW)
	}

	w.savedRoutes = nil
	return nil
}

func (w *windowsRouteManager) GetDefaultGateway() (netip.Addr, error) {
	cmd := exec.Command("route", "PRINT", "0.0.0.0")
	output, err := cmd.Output()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("route print: %w", err)
	}

	// Parse output to find default gateway
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		// Look for line with "0.0.0.0 0.0.0.0" (destination and mask for default route)
		if len(fields) >= 3 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
			addr, err := netip.ParseAddr(fields[2])
			if err == nil {
				return addr, nil
			}
		}
	}

	return netip.Addr{}, fmt.Errorf("no default gateway found")
}

// windowsFirewallManager implements FirewallManager for Windows using netsh advfirewall.
type windowsFirewallManager struct {
	rulesAdded bool
	ruleName   string
}

func newWindowsFirewallManager() (FirewallManager, error) {
	// Check if netsh is available
	if _, err := exec.LookPath("netsh.exe"); err != nil {
		return nil, fmt.Errorf("netsh.exe not found: %w", err)
	}

	return &windowsFirewallManager{
		rulesAdded: false,
		ruleName:   "i2plan-killswitch",
	}, nil
}

func (w *windowsFirewallManager) BlockNonVPN(allowedInterface string) error {
	// For MVP, this is a placeholder
	// Full implementation would use netsh advfirewall to add rules
	log.Info("windows: would block non-VPN traffic", "interface", allowedInterface)
	w.rulesAdded = true
	return nil
}

func (w *windowsFirewallManager) Restore() error {
	if !w.rulesAdded {
		return nil
	}

	log.Info("windows: would restore firewall rules")
	w.rulesAdded = false
	return nil
}
