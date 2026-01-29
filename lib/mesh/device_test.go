package mesh

import (
	"net/netip"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestNewDevice(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey() error = %v", err)
	}

	tunnelIP := AllocateTunnelIP(key.PublicKey())

	dev, err := NewDevice(DeviceConfig{
		PrivateKey: key,
		TunnelIP:   tunnelIP,
		Subnet:     TunnelSubnet,
		MTU:        1280,
	})
	if err != nil {
		t.Fatalf("NewDevice() error = %v", err)
	}
	defer dev.Close()

	if dev.TunnelIP() != tunnelIP {
		t.Errorf("TunnelIP() = %v, want %v", dev.TunnelIP(), tunnelIP)
	}

	if dev.Subnet() != TunnelSubnet {
		t.Errorf("Subnet() = %v, want %v", dev.Subnet(), TunnelSubnet)
	}

	if dev.PublicKey() != key.PublicKey() {
		t.Errorf("PublicKey() mismatch")
	}

	if dev.PeerCount() != 0 {
		t.Errorf("PeerCount() = %d, want 0", dev.PeerCount())
	}
}

func TestDevice_AddRemovePeer(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	tunnelIP := AllocateTunnelIP(key.PublicKey())

	dev, err := NewDevice(DeviceConfig{
		PrivateKey: key,
		TunnelIP:   tunnelIP,
		Subnet:     TunnelSubnet,
	})
	if err != nil {
		t.Fatalf("NewDevice() error = %v", err)
	}
	defer dev.Close()

	// Generate a peer
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	allowedIPs := []netip.Prefix{netip.PrefixFrom(peerIP, 32)}

	// Add peer
	err = dev.AddPeer(peerKey.PublicKey(), allowedIPs, "")
	if err != nil {
		t.Fatalf("AddPeer() error = %v", err)
	}

	if dev.PeerCount() != 1 {
		t.Errorf("PeerCount() = %d, want 1", dev.PeerCount())
	}

	// Get peer
	peer, ok := dev.GetPeer(peerKey.PublicKey())
	if !ok {
		t.Fatal("GetPeer() not found")
	}
	if peer.PublicKey != peerKey.PublicKey() {
		t.Errorf("peer.PublicKey mismatch")
	}

	// Remove peer
	err = dev.RemovePeer(peerKey.PublicKey())
	if err != nil {
		t.Fatalf("RemovePeer() error = %v", err)
	}

	if dev.PeerCount() != 0 {
		t.Errorf("PeerCount() after remove = %d, want 0", dev.PeerCount())
	}
}

func TestDevice_ListPeers(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	tunnelIP := AllocateTunnelIP(key.PublicKey())

	dev, err := NewDevice(DeviceConfig{
		PrivateKey: key,
		TunnelIP:   tunnelIP,
		Subnet:     TunnelSubnet,
	})
	if err != nil {
		t.Fatalf("NewDevice() error = %v", err)
	}
	defer dev.Close()

	// Add multiple peers
	for i := 0; i < 3; i++ {
		peerKey, _ := wgtypes.GeneratePrivateKey()
		peerIP := AllocateTunnelIP(peerKey.PublicKey())
		dev.AddPeer(peerKey.PublicKey(), []netip.Prefix{netip.PrefixFrom(peerIP, 32)}, "")
	}

	peers := dev.ListPeers()
	if len(peers) != 3 {
		t.Errorf("ListPeers() returned %d peers, want 3", len(peers))
	}
}

func TestDevice_UpdatePeerEndpoint(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	tunnelIP := AllocateTunnelIP(key.PublicKey())

	dev, err := NewDevice(DeviceConfig{
		PrivateKey: key,
		TunnelIP:   tunnelIP,
		Subnet:     TunnelSubnet,
	})
	if err != nil {
		t.Fatalf("NewDevice() error = %v", err)
	}
	defer dev.Close()

	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())

	// Add peer without endpoint
	dev.AddPeer(peerKey.PublicKey(), []netip.Prefix{netip.PrefixFrom(peerIP, 32)}, "")

	// Update endpoint - note: this would normally be an I2P endpoint
	// For testing we just verify the API works
	err = dev.UpdatePeerEndpoint(peerKey.PublicKey(), "127.0.0.1:51820")
	if err != nil {
		t.Fatalf("UpdatePeerEndpoint() error = %v", err)
	}

	peer, _ := dev.GetPeer(peerKey.PublicKey())
	if peer.Endpoint != "127.0.0.1:51820" {
		t.Errorf("peer.Endpoint = %v, want 127.0.0.1:51820", peer.Endpoint)
	}
}

func TestDevice_Close(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	tunnelIP := AllocateTunnelIP(key.PublicKey())

	dev, err := NewDevice(DeviceConfig{
		PrivateKey: key,
		TunnelIP:   tunnelIP,
		Subnet:     TunnelSubnet,
	})
	if err != nil {
		t.Fatalf("NewDevice() error = %v", err)
	}

	if dev.IsClosed() {
		t.Error("IsClosed() = true before close")
	}

	dev.Close()

	if !dev.IsClosed() {
		t.Error("IsClosed() = false after close")
	}

	// Double close should be safe
	err = dev.Close()
	if err != nil {
		t.Errorf("double Close() error = %v", err)
	}
}

func TestDevice_InvalidConfig(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()

	tests := []struct {
		name string
		cfg  DeviceConfig
	}{
		{
			name: "invalid tunnel IP",
			cfg: DeviceConfig{
				PrivateKey: key,
				TunnelIP:   netip.Addr{},
				Subnet:     TunnelSubnet,
			},
		},
		{
			name: "invalid subnet",
			cfg: DeviceConfig{
				PrivateKey: key,
				TunnelIP:   netip.MustParseAddr("10.42.1.1"),
				Subnet:     netip.Prefix{},
			},
		},
		{
			name: "IP not in subnet",
			cfg: DeviceConfig{
				PrivateKey: key,
				TunnelIP:   netip.MustParseAddr("192.168.1.1"),
				Subnet:     TunnelSubnet,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDevice(tt.cfg)
			if err == nil {
				t.Error("NewDevice() expected error, got nil")
			}
		})
	}
}

func TestDevice_OperationsAfterClose(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	tunnelIP := AllocateTunnelIP(key.PublicKey())

	dev, _ := NewDevice(DeviceConfig{
		PrivateKey: key,
		TunnelIP:   tunnelIP,
		Subnet:     TunnelSubnet,
	})
	dev.Close()

	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())

	err := dev.AddPeer(peerKey.PublicKey(), []netip.Prefix{netip.PrefixFrom(peerIP, 32)}, "")
	if err == nil {
		t.Error("AddPeer() after close should fail")
	}

	err = dev.RemovePeer(peerKey.PublicKey())
	if err == nil {
		t.Error("RemovePeer() after close should fail")
	}

	err = dev.UpdatePeerEndpoint(peerKey.PublicKey(), "127.0.0.1:51820")
	if err == nil {
		t.Error("UpdatePeerEndpoint() after close should fail")
	}
}
