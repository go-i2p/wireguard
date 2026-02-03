//go:build freebsd || openbsd || netbsd

package core

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
)

// bsdRouteManager implements RouteManager for BSD systems using route(8) command.
type bsdRouteManager struct {
	savedRoutes []savedRoute
	originalGW  netip.Addr
}

type savedRoute struct {
	dest    netip.Prefix
	gateway netip.Addr
}

func newBSDRouteManager() (RouteManager, error) {
	// Check if route command is available
	if _, err := exec.LookPath("route"); err != nil {
		return nil, fmt.Errorf("route command not found: %w", err)
	}

	return &bsdRouteManager{
		savedRoutes: make([]savedRoute, 0),
	}, nil
}

func (b *bsdRouteManager) SaveDefaultRoute() error {
	gw, err := b.GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("get default gateway: %w", err)
	}
	b.originalGW = gw
	log.Debug("saved default gateway", "gateway", gw)
	return nil
}

func (b *bsdRouteManager) AddRoute(destination netip.Prefix, gateway netip.Addr) error {
	// Use route add command
	cmd := exec.Command("route", "-n", "add", "-net", destination.String(), gateway.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add: %w (output: %s)", err, string(output))
	}

	b.savedRoutes = append(b.savedRoutes, savedRoute{
		dest:    destination,
		gateway: gateway,
	})
	log.Debug("added route", "dest", destination, "gateway", gateway)
	return nil
}

func (b *bsdRouteManager) SetDefaultRoute(gateway netip.Addr) error {
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

func (b *bsdRouteManager) RestoreRoutes() error {
	// Remove added routes in reverse order
	for i := len(b.savedRoutes) - 1; i >= 0; i-- {
		route := b.savedRoutes[i]
		cmd := exec.Command("route", "-n", "delete", "-net", route.dest.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Warn("failed to remove route", "dest", route.dest, "error", err, "output", string(output))
		}
	}

	// Restore original default gateway
	if b.originalGW.IsValid() {
		cmd := exec.Command("route", "-n", "delete", "default")
		_ = cmd.Run()

		cmd = exec.Command("route", "-n", "add", "default", b.originalGW.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore default gateway: %w (output: %s)", err, string(output))
		}
		log.Debug("restored default gateway", "gateway", b.originalGW)
	}

	b.savedRoutes = nil
	return nil
}

func (b *bsdRouteManager) GetDefaultGateway() (netip.Addr, error) {
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

// bsdFirewallManager implements FirewallManager for BSD systems using pf (packet filter).
type bsdFirewallManager struct {
	rulesAdded bool
}

func newBSDFirewallManager() (FirewallManager, error) {
	// Check if pfctl is available
	if _, err := exec.LookPath("pfctl"); err != nil {
		return nil, fmt.Errorf("pfctl not found: %w", err)
	}

	return &bsdFirewallManager{
		rulesAdded: false,
	}, nil
}

func (b *bsdFirewallManager) BlockNonVPN(allowedInterface string) error {
	// For MVP, this is a placeholder
	// Full implementation would use pfctl to add rules
	log.Info("bsd: would block non-VPN traffic", "interface", allowedInterface)
	b.rulesAdded = true
	return nil
}

func (b *bsdFirewallManager) Restore() error {
	if !b.rulesAdded {
		return nil
	}

	log.Info("bsd: would restore firewall rules")
	b.rulesAdded = false
	return nil
}

// Route manager stubs for other platforms
func newLinuxRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("linux not supported on bsd build")
}

func newDarwinRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("darwin not supported on bsd build")
}

func newWindowsRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("windows not supported on bsd build")
}

// Firewall manager stubs for other platforms
func newLinuxFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("linux not supported on bsd build")
}

func newDarwinFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("darwin not supported on bsd build")
}

func newWindowsFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("windows not supported on bsd build")
}
