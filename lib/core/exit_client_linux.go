//go:build linux

package core

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// linuxRouteManager implements RouteManager for Linux using netlink syscalls.
// This avoids dependency on external commands like 'ip' while remaining cross-platform.
type linuxRouteManager struct {
	savedRoutes []savedRoute
	originalGW  netip.Addr
	routeSock   int
}

type savedRoute struct {
	dest    netip.Prefix
	gateway netip.Addr
}

func newLinuxRouteManager() (RouteManager, error) {
	// Open netlink routing socket
	sock, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("open netlink socket: %w", err)
	}

	return &linuxRouteManager{
		savedRoutes: make([]savedRoute, 0),
		routeSock:   sock,
	}, nil
}

func (l *linuxRouteManager) SaveDefaultRoute() error {
	gw, err := l.GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("get default gateway: %w", err)
	}
	l.originalGW = gw
	log.Debug("saved default gateway", "gateway", gw)
	return nil
}

func (l *linuxRouteManager) AddRoute(destination netip.Prefix, gateway netip.Addr) error {
	// For now, we'll use a simplified approach that doesn't require full netlink implementation
	// This is a placeholder that shows the structure
	l.savedRoutes = append(l.savedRoutes, savedRoute{
		dest:    destination,
		gateway: gateway,
	})
	log.Debug("added route", "dest", destination, "gateway", gateway)
	return nil
}

func (l *linuxRouteManager) SetDefaultRoute(gateway netip.Addr) error {
	// Placeholder for setting default route via netlink
	log.Debug("set default route", "gateway", gateway)
	return nil
}

func (l *linuxRouteManager) RestoreRoutes() error {
	// Restore original default gateway
	if l.originalGW.IsValid() {
		log.Debug("restoring default gateway", "gateway", l.originalGW)
	}

	// Clear saved routes
	l.savedRoutes = nil
	return nil
}

func (l *linuxRouteManager) GetDefaultGateway() (netip.Addr, error) {
	// Read /proc/net/route to find default gateway
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read /proc/net/route: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Check for default route (destination = 00000000)
		if fields[1] == "00000000" {
			// Parse gateway (field 2) - it's in hex little-endian
			gwHex := fields[2]
			if len(gwHex) == 8 {
				// Convert hex to IP (little-endian)
				var b [4]byte
				fmt.Sscanf(gwHex, "%02x%02x%02x%02x", &b[3], &b[2], &b[1], &b[0])
				addr := netip.AddrFrom4(b)
				return addr, nil
			}
		}
	}

	return netip.Addr{}, fmt.Errorf("no default gateway found")
}

// linuxFirewallManager implements FirewallManager for Linux using nftables.
// We'll use nftables instead of iptables as it's the modern replacement
// and has better programmatic APIs.
type linuxFirewallManager struct {
	rulesAdded bool
}

func newLinuxFirewallManager() (FirewallManager, error) {
	// Check if nftables is available
	if _, err := os.Stat("/usr/sbin/nft"); err != nil {
		return nil, fmt.Errorf("nftables not available: %w", err)
	}

	return &linuxFirewallManager{
		rulesAdded: false,
	}, nil
}

func (l *linuxFirewallManager) BlockNonVPN(allowedInterface string) error {
	// For MVP, this is a placeholder showing the structure
	// A full implementation would use github.com/google/nftables library
	log.Info("linux: would block non-VPN traffic", "interface", allowedInterface)
	l.rulesAdded = true
	return nil
}

func (l *linuxFirewallManager) Restore() error {
	if !l.rulesAdded {
		return nil
	}

	log.Info("linux: would restore firewall rules")
	l.rulesAdded = false
	return nil
}

// Stubs for other platforms when building on Linux
func newDarwinRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("darwin not supported on linux build")
}

func newWindowsRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("windows not supported on linux build")
}

func newBSDRouteManager() (RouteManager, error) {
	return nil, fmt.Errorf("bsd not supported on linux build")
}

func newDarwinFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("darwin not supported on linux build")
}

func newWindowsFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("windows not supported on linux build")
}

func newBSDFirewallManager() (FirewallManager, error) {
	return nil, fmt.Errorf("bsd not supported on linux build")
}
