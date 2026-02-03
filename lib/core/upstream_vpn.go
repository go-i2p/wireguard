// Package core provides upstream VPN detection for exit nodes.
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
// a list of detected VPNs. This function checks for known VPN interface
// naming patterns used by popular VPN providers and protocols.
//
// Detected patterns:
//   - tun0-9: Common OpenVPN and generic tunnel interfaces
//   - wg*: WireGuard interfaces (mullvad, proton, etc.)
//   - proton*: ProtonVPN interfaces
//   - nordlynx: NordVPN WireGuard interface
//   - tailscale*: Tailscale mesh VPN
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

	// Known VPN interface patterns to check
	vpnPatterns := []string{
		"tun0", "tun1", "tun2", "tun3", "tun4", "tun5", "tun6", "tun7", "tun8", "tun9",
		"wg-mullvad", "wg-quick", "wg0", "wg1", "wg2",
		"proton0", "proton1", "protonvpn",
		"nordlynx",
		"tailscale0",
	}

	var vpns []*UpstreamVPN

	// Check each pattern against actual interfaces
	for _, pattern := range vpnPatterns {
		for _, iface := range interfaces {
			// Match exact name or prefix for flexible matching
			if iface.Name == pattern || strings.HasPrefix(iface.Name, pattern) {
				vpn := &UpstreamVPN{
					Interface: iface.Name,
					Type:      detectVPNType(iface.Name),
					IsActive:  isInterfaceUp(iface),
				}
				vpns = append(vpns, vpn)
				break // Found this pattern, move to next
			}
		}
	}

	return vpns, nil
}

// detectVPNType attempts to identify the VPN type based on interface name patterns.
// This is heuristic-based and may not always be accurate, but provides useful
// information for route selection and troubleshooting.
func detectVPNType(interfaceName string) string {
	name := strings.ToLower(interfaceName)

	// WireGuard detection
	if strings.HasPrefix(name, "wg") || strings.Contains(name, "wireguard") ||
		name == "nordlynx" || strings.Contains(name, "mullvad") {
		return "wireguard"
	}

	// OpenVPN detection (tun/tap interfaces are commonly used by OpenVPN)
	if strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "tap") {
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
