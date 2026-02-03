//go:build windows

package core

// getPlatformVPNPatterns returns Windows-specific VPN interface naming patterns.
//
// Windows VPN interface naming is more complex than Unix-like systems:
//   - Interfaces often have friendly names like "Ethernet", "Wi-Fi"
//   - VPN adapters may include provider names in the interface name
//   - TAP adapters created by OpenVPN have "TAP-Windows" in the name
//   - WireGuard creates interfaces with "WireGuard Tunnel" or custom names
//
// Note: Windows interface detection is less reliable due to variable naming.
// This list includes common patterns, but may need user configuration for
// specific VPN providers.
//
// LIMITATION: Full Windows support requires querying adapter properties via
// Windows API (GetAdaptersInfo). For MVP, we match common name patterns.
func getPlatformVPNPatterns() []string {
	return []string{
		// WireGuard patterns
		"WireGuard*",
		"wg*",

		// OpenVPN patterns
		"TAP-Windows*",
		"OpenVPN*",
		"TAP*",

		// Provider-specific patterns (case-insensitive on Windows)
		"Mullvad*",
		"ProtonVPN*",
		"NordVPN*",
		"Tailscale*",

		// Generic VPN adapter names
		"VPN*",
		"Tunnel*",
	}
}
