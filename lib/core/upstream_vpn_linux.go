//go:build linux

package core

// getPlatformVPNPatterns returns Linux-specific VPN interface naming patterns.
//
// Linux VPN interfaces typically follow these naming conventions:
//   - tunX: OpenVPN and generic tunnel interfaces (tun0-tun9)
//   - tapX: TAP interfaces used by some VPNs
//   - wgX: Generic WireGuard interfaces (wg0-wg9)
//   - wg-*: Named WireGuard interfaces (wg-mullvad, wg-quick)
//   - protonX: ProtonVPN interfaces
//   - nordlynx: NordVPN's WireGuard implementation
//   - tailscale*: Tailscale mesh VPN interfaces
func getPlatformVPNPatterns() []string {
	return []string{
		// OpenVPN/generic tunnel interfaces
		"tun0", "tun1", "tun2", "tun3", "tun4",
		"tun5", "tun6", "tun7", "tun8", "tun9",
		"tap0", "tap1", "tap2", "tap3", "tap4",

		// WireGuard interfaces
		"wg0", "wg1", "wg2", "wg3", "wg4",
		"wg-mullvad", "wg-quick", "wg-*",

		// Provider-specific interfaces
		"proton0", "proton1", "protonvpn",
		"nordlynx",
		"tailscale*",

		// Additional common patterns
		"vpn*", // Generic VPN interfaces
	}
}
