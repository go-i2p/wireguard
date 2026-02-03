// Package core provides upstream VPN detection for exit nodes.
//
// CROSS-PLATFORM DESIGN:
// VPN interface detection is platform-specific due to different naming conventions:
//   - Linux/Unix: tun0, wg0, etc.
//   - macOS: utun0, utun1, etc. (plus standard tun/wg)
//   - Windows: adapter GUIDs or friendly names
//   - BSD: tun0, wg0 (similar to Linux)
//
// The main file contains common types and cross-platform helpers.
// Platform-specific detection logic is in upstream_vpn_*.go files with build tags.
package core

import (
	"fmt"
	"net"
	"strings"
)

// UpstreamVPN represents an upstream VPN connection detected on the system.
// Exit nodes can route mesh traffic through these VPNs to provide additional
// privacy and location diversity.
type UpstreamVPN struct {
	// Interface is the network interface name (e.g., "tun0", "wg-mullvad")
	Interface string
	// Type identifies the VPN type: "wireguard", "openvpn", "unknown"
	Type string
	// PublicIP is the public IP address seen from this interface (if detectable)
	PublicIP string
	// IsActive indicates whether the interface is currently up and usable
	IsActive bool
}

// DetectUpstreamVPNs scans the system for common VPN interfaces and returns
// a list of detected VPNs. This function uses platform-specific VPN interface
// patterns defined in upstream_vpn_*.go files.
//
// Platform-specific patterns:
//   - Linux: tun*, wg*, proton*, nordlynx, tailscale*
//   - macOS: utun*, tun*, wg*, tailscale*
//   - Windows: adapter names matching VPN patterns
//   - BSD: tun*, wg*, (similar to Linux)
//
// This function only detects interfaces; it does not verify they are
// actually connected to the internet or functioning correctly.
// Use VerifyUpstreamVPN() for health checks.
func DetectUpstreamVPNs() ([]*UpstreamVPN, error) {
	// Get all network interfaces on the system
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}

	// Get platform-specific VPN patterns
	vpnPatterns := getPlatformVPNPatterns()

	vpns := make([]*UpstreamVPN, 0) // Initialize as empty slice, not nil
	seen := make(map[string]bool)   // Prevent duplicates

	// Check each pattern against actual interfaces
	for _, pattern := range vpnPatterns {
		for _, iface := range interfaces {
			// Skip if already added
			if seen[iface.Name] {
				continue
			}

			// Match exact name or prefix for flexible matching
			if matchesVPNPattern(iface.Name, pattern) {
				vpn := &UpstreamVPN{
					Interface: iface.Name,
					Type:      detectVPNType(iface.Name),
					IsActive:  isInterfaceUp(iface),
				}
				vpns = append(vpns, vpn)
				seen[iface.Name] = true
			}
		}
	}

	return vpns, nil
}

// matchesVPNPattern checks if an interface name matches a VPN pattern.
// Supports exact matches and prefix matching (pattern ending with *).
func matchesVPNPattern(interfaceName, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		// Prefix matching
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(interfaceName, prefix)
	}
	// Exact match
	return interfaceName == pattern
}

// detectVPNType attempts to identify the VPN type based on interface name patterns.
// This is heuristic-based and may not always be accurate, but provides useful
// information for route selection and troubleshooting.
//
// CROSS-PLATFORM: This function works with interface names from any platform.
func detectVPNType(interfaceName string) string {
	name := strings.ToLower(interfaceName)

	// WireGuard detection
	if strings.HasPrefix(name, "wg") || strings.Contains(name, "wireguard") ||
		name == "nordlynx" || strings.Contains(name, "mullvad") {
		return "wireguard"
	}

	// OpenVPN detection (tun/tap interfaces are commonly used by OpenVPN)
	// Note: utun on macOS can be either WireGuard or OpenVPN
	if strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "tap") ||
		strings.HasPrefix(name, "utun") {
		return "openvpn"
	}

	// ProtonVPN detection
	if strings.Contains(name, "proton") {
		return "openvpn" // ProtonVPN typically uses OpenVPN
	}

	// Tailscale detection
	if strings.Contains(name, "tailscale") {
		return "wireguard" // Tailscale uses WireGuard under the hood
	}

	return "unknown"
}

// isInterfaceUp checks if a network interface is up and active.
// An interface is considered active if it has the UP flag set.
func isInterfaceUp(iface net.Interface) bool {
	// Check if the interface is up (IFF_UP flag)
	return iface.Flags&net.FlagUp != 0
}

// InterfaceExists checks if a network interface with the given name exists on the system.
// Returns true if the interface exists, false otherwise.
// Returns an error only if there's a system error reading interfaces.
func InterfaceExists(name string) (bool, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		// Check if the error is "no such network interface"
		// In Go, this is represented as an opError with "no such network interface"
		if strings.Contains(err.Error(), "no such network interface") {
			return false, nil
		}
		// Other errors (permission denied, etc.) are real errors
		return false, fmt.Errorf("check interface %s: %w", name, err)
	}
	return iface != nil, nil
}

// GetInterfaceIPs returns all IP addresses assigned to the given interface.
// Returns nil slice if the interface has no IPs or doesn't exist.
func GetInterfaceIPs(name string) ([]net.IP, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("get interface %s: %w", name, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("get addresses for %s: %w", name, err)
	}

	var ips []net.IP
	for _, addr := range addrs {
		// Extract IP from CIDR notation (e.g., "10.0.0.1/24" -> "10.0.0.1")
		ipNet, ok := addr.(*net.IPNet)
		if ok {
			ips = append(ips, ipNet.IP)
		}
	}

	return ips, nil
}
