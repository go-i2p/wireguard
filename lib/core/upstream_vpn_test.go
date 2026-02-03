package core

import (
	"net"
	"testing"
)

// TestDetectUpstreamVPNs tests the basic VPN detection functionality.
// It verifies that the function can execute without errors and returns
// a valid (possibly empty) list of VPNs.
func TestDetectUpstreamVPNs(t *testing.T) {
	vpns, err := DetectUpstreamVPNs()
	if err != nil {
		t.Fatalf("DetectUpstreamVPNs() failed: %v", err)
	}

	// Should return a non-nil slice even if empty
	if vpns == nil {
		t.Error("DetectUpstreamVPNs() returned nil slice")
	}

	// Verify structure of each detected VPN
	for i, vpn := range vpns {
		if vpn.Interface == "" {
			t.Errorf("VPN %d: Interface name is empty", i)
		}
		if vpn.Type == "" {
			t.Errorf("VPN %d: Type is empty", i)
		}
		// IsActive and PublicIP can be any value, just check they exist
		t.Logf("Detected VPN %d: %s (type: %s, active: %v)", i, vpn.Interface, vpn.Type, vpn.IsActive)
	}
}

// TestDetectVPNType tests the VPN type detection logic.
func TestDetectVPNType(t *testing.T) {
	tests := []struct {
		name          string
		interfaceName string
		wantType      string
	}{
		// WireGuard interfaces
		{
			name:          "WireGuard standard interface",
			interfaceName: "wg0",
			wantType:      "wireguard",
		},
		{
			name:          "Mullvad WireGuard interface",
			interfaceName: "wg-mullvad",
			wantType:      "wireguard",
		},
		{
			name:          "NordVPN WireGuard interface",
			interfaceName: "nordlynx",
			wantType:      "wireguard",
		},
		{
			name:          "Tailscale interface",
			interfaceName: "tailscale0",
			wantType:      "wireguard",
		},
		{
			name:          "WireGuard quick interface",
			interfaceName: "wg-quick",
			wantType:      "wireguard",
		},

		// OpenVPN interfaces
		{
			name:          "OpenVPN tun0",
			interfaceName: "tun0",
			wantType:      "openvpn",
		},
		{
			name:          "OpenVPN tun1",
			interfaceName: "tun1",
			wantType:      "openvpn",
		},
		{
			name:          "OpenVPN tap0",
			interfaceName: "tap0",
			wantType:      "openvpn",
		},
		{
			name:          "ProtonVPN interface",
			interfaceName: "proton0",
			wantType:      "openvpn",
		},
		{
			name:          "ProtonVPN alternate",
			interfaceName: "protonvpn",
			wantType:      "openvpn",
		},

		// Unknown interfaces
		{
			name:          "Regular ethernet",
			interfaceName: "eth0",
			wantType:      "unknown",
		},
		{
			name:          "WiFi interface",
			interfaceName: "wlan0",
			wantType:      "unknown",
		},
		{
			name:          "Loopback interface",
			interfaceName: "lo",
			wantType:      "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType := detectVPNType(tt.interfaceName)
			if gotType != tt.wantType {
				t.Errorf("detectVPNType(%q) = %q, want %q", tt.interfaceName, gotType, tt.wantType)
			}
		})
	}
}

// TestDetectVPNTypeCaseInsensitive verifies that VPN type detection
// works regardless of interface name casing.
func TestDetectVPNTypeCaseInsensitive(t *testing.T) {
	testCases := []struct {
		name  string
		iface string
		want  string
	}{
		{"uppercase WG", "WG0", "wireguard"},
		{"mixed case WG", "Wg0", "wireguard"},
		{"uppercase TUN", "TUN0", "openvpn"},
		{"mixed case TUN", "Tun0", "openvpn"},
		{"uppercase PROTON", "PROTON0", "openvpn"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectVPNType(tc.iface)
			if got != tc.want {
				t.Errorf("detectVPNType(%q) = %q, want %q", tc.iface, got, tc.want)
			}
		})
	}
}

// TestIsInterfaceUp tests the interface status checking.
func TestIsInterfaceUp(t *testing.T) {
	// Get all real interfaces on the system
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Failed to get interfaces: %v", err)
	}

	// At least one interface should exist (loopback)
	if len(interfaces) == 0 {
		t.Skip("No network interfaces found on system")
	}

	// Test with a real interface
	for _, iface := range interfaces {
		isUp := isInterfaceUp(iface)
		hasUpFlag := iface.Flags&net.FlagUp != 0

		if isUp != hasUpFlag {
			t.Errorf("isInterfaceUp(%s) = %v, but UP flag is %v", iface.Name, isUp, hasUpFlag)
		}

		t.Logf("Interface %s: up=%v flags=%v", iface.Name, isUp, iface.Flags)
	}
}

// TestInterfaceExists tests checking for interface existence.
func TestInterfaceExists(t *testing.T) {
	// Test with a known interface (loopback should always exist)
	exists, err := InterfaceExists("lo")
	if err != nil {
		t.Fatalf("InterfaceExists(lo) returned error: %v", err)
	}
	if !exists {
		t.Error("InterfaceExists(lo) = false, loopback interface should exist")
	}

	// Test with a non-existent interface
	exists, err = InterfaceExists("nonexistent12345xyz")
	if err != nil {
		t.Fatalf("InterfaceExists(nonexistent) returned error: %v", err)
	}
	if exists {
		t.Error("InterfaceExists(nonexistent12345xyz) = true, should not exist")
	}
}

// TestInterfaceExistsWithRealInterfaces tests with actual system interfaces.
func TestInterfaceExistsWithRealInterfaces(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Failed to get interfaces: %v", err)
	}

	// Test that existing interfaces are detected
	for _, iface := range interfaces {
		exists, err := InterfaceExists(iface.Name)
		if err != nil {
			t.Errorf("InterfaceExists(%s) returned error: %v", iface.Name, err)
		}
		if !exists {
			t.Errorf("InterfaceExists(%s) = false, but interface exists", iface.Name)
		}
	}
}

// TestGetInterfaceIPs tests retrieving IP addresses from interfaces.
func TestGetInterfaceIPs(t *testing.T) {
	// Test with loopback interface
	ips, err := GetInterfaceIPs("lo")
	if err != nil {
		t.Fatalf("GetInterfaceIPs(lo) failed: %v", err)
	}

	// Loopback should have at least one IP (127.0.0.1 or ::1)
	if len(ips) == 0 {
		t.Error("GetInterfaceIPs(lo) returned no IPs, expected at least loopback")
	}

	// Verify IPs are valid
	for _, ip := range ips {
		if ip == nil {
			t.Error("GetInterfaceIPs(lo) returned nil IP")
		}
		t.Logf("Loopback IP: %s", ip)
	}
}

// TestGetInterfaceIPsNonExistent tests error handling for non-existent interfaces.
func TestGetInterfaceIPsNonExistent(t *testing.T) {
	_, err := GetInterfaceIPs("nonexistent12345xyz")
	if err == nil {
		t.Error("GetInterfaceIPs(nonexistent) should return error")
	}
}

// TestGetInterfaceIPsAllInterfaces tests IP retrieval for all system interfaces.
func TestGetInterfaceIPsAllInterfaces(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Failed to get interfaces: %v", err)
	}

	for _, iface := range interfaces {
		ips, err := GetInterfaceIPs(iface.Name)
		if err != nil {
			t.Errorf("GetInterfaceIPs(%s) failed: %v", iface.Name, err)
			continue
		}

		// Some interfaces may not have IPs (down interfaces, virtual, etc.)
		// Just log what we find
		if len(ips) > 0 {
			t.Logf("Interface %s has %d IP(s):", iface.Name, len(ips))
			for _, ip := range ips {
				t.Logf("  - %s", ip)
			}
		} else {
			t.Logf("Interface %s has no IPs", iface.Name)
		}
	}
}

// TestUpstreamVPNStructure tests the UpstreamVPN struct initialization.
func TestUpstreamVPNStructure(t *testing.T) {
	vpn := &UpstreamVPN{
		Interface: "wg0",
		Type:      "wireguard",
		PublicIP:  "1.2.3.4",
		IsActive:  true,
	}

	if vpn.Interface != "wg0" {
		t.Errorf("Interface = %q, want %q", vpn.Interface, "wg0")
	}
	if vpn.Type != "wireguard" {
		t.Errorf("Type = %q, want %q", vpn.Type, "wireguard")
	}
	if vpn.PublicIP != "1.2.3.4" {
		t.Errorf("PublicIP = %q, want %q", vpn.PublicIP, "1.2.3.4")
	}
	if !vpn.IsActive {
		t.Error("IsActive = false, want true")
	}
}

// TestDetectUpstreamVPNsNoMatches tests behavior when no VPN interfaces exist.
func TestDetectUpstreamVPNsNoMatches(t *testing.T) {
	// This test documents expected behavior rather than forcing it
	// In a system without VPNs, we should get an empty list, not an error
	vpns, err := DetectUpstreamVPNs()
	if err != nil {
		t.Fatalf("DetectUpstreamVPNs() should not error on no matches: %v", err)
	}

	// Result should be a valid empty slice, not nil
	if vpns == nil {
		t.Error("DetectUpstreamVPNs() returned nil, want empty slice")
	}

	t.Logf("Detected %d VPN interface(s)", len(vpns))
}

// BenchmarkDetectUpstreamVPNs benchmarks the VPN detection performance.
func BenchmarkDetectUpstreamVPNs(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = DetectUpstreamVPNs()
	}
}

// BenchmarkDetectVPNType benchmarks the VPN type detection.
func BenchmarkDetectVPNType(b *testing.B) {
	interfaces := []string{"wg0", "tun0", "eth0", "proton0", "nordlynx"}
	for i := 0; i < b.N; i++ {
		for _, iface := range interfaces {
			_ = detectVPNType(iface)
		}
	}
}

// BenchmarkInterfaceExists benchmarks interface existence checks.
func BenchmarkInterfaceExists(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = InterfaceExists("lo")
	}
}
