package mesh

import (
	"net/netip"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestMessageType_String(t *testing.T) {
	tests := []struct {
		msgType MessageType
		want    string
	}{
		{MsgHandshakeInit, "HandshakeInit"},
		{MsgHandshakeResponse, "HandshakeResponse"},
		{MsgHandshakeComplete, "HandshakeComplete"},
		{MsgPeerAnnounce, "PeerAnnounce"},
		{MsgPeerList, "PeerList"},
		{MsgRouteUpdate, "RouteUpdate"},
		{MsgPeerLeave, "PeerLeave"},
		{MessageType(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.msgType.String(); got != tt.want {
				t.Errorf("MessageType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeDecodeMessage(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey() error = %v", err)
	}

	init := &HandshakeInit{
		I2PDest:     "test.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    "10.42.1.1",
		NetworkID:   "test-network",
		AuthToken:   []byte("secret-token"),
		NodeID:      "node-123",
	}

	// Encode
	data, err := EncodeMessage(MsgHandshakeInit, init)
	if err != nil {
		t.Fatalf("EncodeMessage() error = %v", err)
	}

	// Decode envelope
	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if msg.Type != MsgHandshakeInit {
		t.Errorf("Message.Type = %v, want %v", msg.Type, MsgHandshakeInit)
	}

	// Decode payload
	payload, err := DecodePayload[HandshakeInit](msg)
	if err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}

	if payload.I2PDest != init.I2PDest {
		t.Errorf("I2PDest = %v, want %v", payload.I2PDest, init.I2PDest)
	}
	if payload.WGPublicKey != init.WGPublicKey {
		t.Errorf("WGPublicKey = %v, want %v", payload.WGPublicKey, init.WGPublicKey)
	}
	if payload.TunnelIP != init.TunnelIP {
		t.Errorf("TunnelIP = %v, want %v", payload.TunnelIP, init.TunnelIP)
	}
}

func TestAllocateTunnelIP(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey() error = %v", err)
	}

	ip := AllocateTunnelIP(key.PublicKey())

	// Should be in default subnet
	if !TunnelSubnet.Contains(ip) {
		t.Errorf("AllocateTunnelIP() = %v, not in subnet %v", ip, TunnelSubnet)
	}

	// Should be deterministic
	ip2 := AllocateTunnelIP(key.PublicKey())
	if ip != ip2 {
		t.Errorf("AllocateTunnelIP() not deterministic: %v != %v", ip, ip2)
	}

	// Should be IPv4
	if !ip.Is4() {
		t.Errorf("AllocateTunnelIP() = %v, not IPv4", ip)
	}

	// Last octet should not be 0 or 255
	addr := ip.As4()
	if addr[3] == 0 || addr[3] == 255 {
		t.Errorf("AllocateTunnelIP() = %v, has reserved last octet", ip)
	}
}

func TestAllocateTunnelIP_DifferentKeys(t *testing.T) {
	key1, _ := wgtypes.GeneratePrivateKey()
	key2, _ := wgtypes.GeneratePrivateKey()

	ip1 := AllocateTunnelIP(key1.PublicKey())
	ip2 := AllocateTunnelIP(key2.PublicKey())

	// Different keys should (almost always) produce different IPs
	// This could theoretically fail with a hash collision, but extremely unlikely
	if ip1 == ip2 {
		t.Logf("Note: collision detected between %v and %v (extremely rare)", ip1, ip2)
	}
}

func TestValidateTunnelIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid IP", "10.42.1.100", false},
		{"valid edge", "10.42.255.1", false},
		{"network address", "10.42.1.0", true},
		{"broadcast address", "10.42.1.255", true},
		{"outside subnet", "192.168.1.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, err := netip.ParseAddr(tt.ip)
			if err != nil {
				t.Fatalf("ParseAddr(%q) error = %v", tt.ip, err)
			}

			err = ValidateTunnelIP(ip, TunnelSubnet)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTunnelIP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPCollision(t *testing.T) {
	key1, _ := wgtypes.GeneratePrivateKey()
	key2, _ := wgtypes.GeneratePrivateKey()

	// Same key should collide with itself
	if !IPCollision(key1.PublicKey(), key1.PublicKey(), TunnelSubnet) {
		t.Error("IPCollision() same key should return true")
	}

	// Different keys should (almost always) not collide
	collision := IPCollision(key1.PublicKey(), key2.PublicKey(), TunnelSubnet)
	t.Logf("IPCollision between different keys: %v", collision)
}
