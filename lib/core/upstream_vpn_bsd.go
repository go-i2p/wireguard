//go:build freebsd || openbsd || netbsd || dragonfly

package core

// getPlatformVPNPatterns returns BSD-specific VPN interface naming patterns.
//
// BSD systems (FreeBSD, OpenBSD, NetBSD, DragonFly BSD) use similar
// naming conventions to Linux:
//   - tunX: OpenVPN and generic tunnel interfaces
//   - tapX: TAP interfaces
//   - wgX: WireGuard interfaces
//
// OpenBSD has additional considerations:
//   - WireGuard is built into the kernel (wg0, wg1, etc.)
//   - tun devices are used by OpenVPN and others
//
// FreeBSD:
//   - Supports both tun and tap devices
//   - WireGuard available via kernel module or wireguard-go
func getPlatformVPNPatterns() []string {
	return []string{
		// Standard tunnel interfaces
		"tun0", "tun1", "tun2", "tun3", "tun4",
		"tun5", "tun6", "tun7", "tun8", "tun9",
		"tap0", "tap1", "tap2", "tap3", "tap4",

		// WireGuard interfaces
		"wg0", "wg1", "wg2", "wg3", "wg4",
		"wg-*",

		// Provider-specific interfaces (less common on BSD)
		"tailscale*",

		// Additional BSD-specific patterns
		"gif*", // Generic tunnel interface on BSD
	}
}
