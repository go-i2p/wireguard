//go:build darwin

package core

// getPlatformVPNPatterns returns macOS-specific VPN interface naming patterns.
//
// macOS VPN interfaces follow these naming conventions:
//   - utunX: macOS native tunnel interfaces used by WireGuard, OpenVPN, IPSec
//   - tunX: Compatible OpenVPN and generic tunnel interfaces
//   - tapX: TAP interfaces (less common on macOS)
//   - wgX: WireGuard interfaces (when using wg-quick or wireguard-go)
//   - pppX: Point-to-Point Protocol interfaces (legacy VPNs)
//
// Note: macOS assigns utun interfaces dynamically, often starting at utun0 or higher.
// The system reserves lower numbers, so VPNs typically get utun3+.
func getPlatformVPNPatterns() []string {
	return []string{
		// macOS-specific utun interfaces (most common)
		"utun0", "utun1", "utun2", "utun3", "utun4",
		"utun5", "utun6", "utun7", "utun8", "utun9",

		// Standard tunnel interfaces
		"tun0", "tun1", "tun2", "tun3", "tun4",
		"tap0", "tap1", "tap2", "tap3", "tap4",

		// WireGuard interfaces
		"wg0", "wg1", "wg2",
		"wg-mullvad", "wg-quick",

		// Provider-specific interfaces
		"nordlynx",
		"tailscale*",

		// Legacy VPN interfaces
		"ppp0", "ppp1", "ppp2",
	}
}
