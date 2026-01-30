package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

// mockSigner implements MessageSigner for testing
type mockSigner struct {
	nodeID       string
	verifyingKey string
	signFunc     func([]byte) []byte
}

func (m *mockSigner) NodeID() string          { return m.nodeID }
func (m *mockSigner) VerifyingKeyHex() string { return m.verifyingKey }
func (m *mockSigner) Sign(data []byte) []byte { return m.signFunc(data) }

func TestEncodeSignedMessage(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	signer := &mockSigner{
		nodeID:       "test-node-123",
		verifyingKey: hex.EncodeToString(pubKey),
		signFunc:     func(data []byte) []byte { return ed25519.Sign(privKey, data) },
	}

	payload := &PeerAnnounce{
		NodeID:    "test-node-123",
		NetworkID: "test-network",
		PeerCount: 5,
	}

	// Encode with signature
	data, err := EncodeSignedMessage(MsgPeerAnnounce, payload, signer)
	if err != nil {
		t.Fatalf("EncodeSignedMessage() error = %v", err)
	}

	// Decode and verify
	msg, err := DecodeAndVerifyMessage(data)
	if err != nil {
		t.Fatalf("DecodeAndVerifyMessage() error = %v", err)
	}

	if !msg.IsSigned() {
		t.Error("message should be signed")
	}

	if msg.SenderID != "test-node-123" {
		t.Errorf("SenderID = %v, want test-node-123", msg.SenderID)
	}
}

func TestMessage_VerifySignature_Invalid(t *testing.T) {
	msg := &Message{
		Type:         MsgPeerAnnounce,
		Payload:      []byte(`{}`),
		VerifyingKey: "abcd1234", // Invalid - too short
		Signature:    "invalid-signature",
	}

	if msg.VerifySignature() {
		t.Error("invalid signature should not verify")
	}
}

func TestMessage_VerifySignature_Unsigned(t *testing.T) {
	msg := &Message{
		Type:    MsgPeerAnnounce,
		Payload: []byte(`{}`),
	}

	// Unsigned messages should verify (backward compatibility)
	if !msg.VerifySignature() {
		t.Error("unsigned message should verify for backward compatibility")
	}

	if msg.IsSigned() {
		t.Error("unsigned message should return false for IsSigned()")
	}
}

// FuzzDecodeMessage tests message decoding with random input to detect panics
// and ensure graceful error handling for malformed data.
func FuzzDecodeMessage(f *testing.F) {
	// Seed corpus with valid messages
	key, _ := wgtypes.GeneratePrivateKey()

	// Seed 1: Valid HandshakeInit
	init := &HandshakeInit{
		I2PDest:     "test.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    "10.42.1.1",
		NetworkID:   "test-network",
		AuthToken:   []byte("secret-token"),
		NodeID:      "node-123",
	}
	if data, err := EncodeMessage(MsgHandshakeInit, init); err == nil {
		f.Add(data)
	}

	// Seed 2: Valid PeerAnnounce
	announce := &PeerAnnounce{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    "10.42.2.1",
		NetworkID:   "test-network",
		NodeID:      "node-456",
		PeerCount:   5,
	}
	if data, err := EncodeMessage(MsgPeerAnnounce, announce); err == nil {
		f.Add(data)
	}

	// Seed 3: Empty JSON object
	f.Add([]byte(`{}`))

	// Seed 4: Invalid JSON
	f.Add([]byte(`{"type":1,"timestamp":"invalid","payload":"not-json"}`))

	// Seed 5: Truncated message
	f.Add([]byte(`{"type":1,"time`))

	// Fuzz target
	f.Fuzz(func(t *testing.T, data []byte) {
		// DecodeMessage should never panic, even with invalid input
		msg, err := DecodeMessage(data)

		// If decoding succeeded, the message should be minimally valid
		if err == nil {
			if msg == nil {
				t.Error("DecodeMessage returned nil message without error")
			}
			// Type should be within valid range or unknown
			// (MessageType.String() handles unknown types)
			_ = msg.Type.String()
		}
	})
}

// FuzzDecodeAndVerifyMessage tests signed message decoding and verification
// with random input to ensure signature verification doesn't panic.
func FuzzDecodeAndVerifyMessage(f *testing.F) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)

	signer := &mockSigner{
		nodeID:       "fuzz-node",
		verifyingKey: hex.EncodeToString(pubKey),
		signFunc:     func(data []byte) []byte { return ed25519.Sign(privKey, data) },
	}

	// Seed 1: Valid signed message
	payload := &PeerAnnounce{
		NodeID:    "fuzz-node",
		NetworkID: "test-network",
		PeerCount: 3,
	}
	if data, err := EncodeSignedMessage(MsgPeerAnnounce, payload, signer); err == nil {
		f.Add(data)
	}

	// Seed 2: Message with invalid signature
	f.Add([]byte(`{"type":4,"timestamp":"2024-01-01T00:00:00Z","payload":"{}","sender_id":"node1","verifying_key":"abcd1234","signature":"invalid"}`))

	// Seed 3: Message with truncated signature
	f.Add([]byte(`{"type":4,"timestamp":"2024-01-01T00:00:00Z","payload":"{}","verifying_key":"` + hex.EncodeToString(pubKey) + `","signature":"ab"}`))

	// Seed 4: Message with wrong key length
	f.Add([]byte(`{"type":4,"timestamp":"2024-01-01T00:00:00Z","payload":"{}","verifying_key":"abc","signature":"def"}`))

	// Fuzz target
	f.Fuzz(func(t *testing.T, data []byte) {
		// DecodeAndVerifyMessage should never panic
		msg, err := DecodeAndVerifyMessage(data)

		// If successful, signature must be valid or message unsigned
		if err == nil {
			if msg == nil {
				t.Error("DecodeAndVerifyMessage returned nil message without error")
			}
			if msg.IsSigned() && !msg.VerifySignature() {
				t.Error("DecodeAndVerifyMessage succeeded but signature verification failed")
			}
		}
	})
}

// FuzzDecodePayload tests payload decoding with random input to ensure
// type-safe unmarshaling handles malformed data gracefully.
func FuzzDecodePayload(f *testing.F) {
	// Seed 1: Valid HandshakeInit payload
	f.Add([]byte(`{"i2p_dest":"test.b32.i2p","wg_public_key":"somekey","tunnel_ip":"10.42.1.1","network_id":"net1","node_id":"node1"}`))

	// Seed 2: Valid PeerAnnounce payload
	f.Add([]byte(`{"i2p_dest":"peer.b32.i2p","wg_public_key":"key2","tunnel_ip":"10.42.2.1","network_id":"net1","node_id":"node2","peer_count":5}`))

	// Seed 3: Empty object
	f.Add([]byte(`{}`))

	// Seed 4: Wrong types
	f.Add([]byte(`{"peer_count":"not-a-number"}`))

	// Seed 5: Unexpected fields
	f.Add([]byte(`{"unknown_field":123,"i2p_dest":"test.b32.i2p"}`))

	// Fuzz target for HandshakeInit
	f.Fuzz(func(t *testing.T, data []byte) {
		msg := &Message{
			Type:    MsgHandshakeInit,
			Payload: json.RawMessage(data),
		}

		// DecodePayload should never panic
		_, err := DecodePayload[HandshakeInit](msg)

		// We don't assert success/failure, just that it doesn't panic
		// and returns a reasonable error when unmarshaling fails
		_ = err
	})
}

// FuzzAllocateTunnelIP tests IP allocation with random WireGuard keys
// to ensure deterministic behavior and no panics.
func FuzzAllocateTunnelIP(f *testing.F) {
	// Seed with some valid WireGuard keys
	for i := 0; i < 5; i++ {
		key, _ := wgtypes.GeneratePrivateKey()
		f.Add(key[:])
	}

	// Seed with edge cases
	allZeros := make([]byte, 32)
	f.Add(allZeros)

	allOnes := make([]byte, 32)
	for i := range allOnes {
		allOnes[i] = 255
	}
	f.Add(allOnes)

	f.Fuzz(func(t *testing.T, keyBytes []byte) {
		// WireGuard keys must be exactly 32 bytes
		if len(keyBytes) != 32 {
			return
		}

		// Create a WireGuard key from the fuzzed bytes
		var key wgtypes.Key
		copy(key[:], keyBytes)

		// AllocateTunnelIP should never panic
		ip := AllocateTunnelIP(key)

		// IP should be in the default subnet
		if !TunnelSubnet.Contains(ip) {
			t.Errorf("AllocateTunnelIP() = %v, not in subnet %v", ip, TunnelSubnet)
		}

		// IP should be IPv4
		if !ip.Is4() {
			t.Errorf("AllocateTunnelIP() = %v, not IPv4", ip)
		}

		// Last octet should not be 0 or 255
		addr := ip.As4()
		if addr[3] == 0 || addr[3] == 255 {
			t.Errorf("AllocateTunnelIP() = %v, has reserved last octet", ip)
		}

		// Should be deterministic - calling again should give same result
		ip2 := AllocateTunnelIP(key)
		if ip != ip2 {
			t.Errorf("AllocateTunnelIP() not deterministic: %v != %v", ip, ip2)
		}
	})
}

// FuzzValidateTunnelIP tests IP validation with random IP addresses
// to ensure robust error handling.
func FuzzValidateTunnelIP(f *testing.F) {
	// Seed with valid IPs
	f.Add([]byte{10, 42, 1, 100})
	f.Add([]byte{10, 42, 255, 1})

	// Seed with invalid IPs
	f.Add([]byte{10, 42, 1, 0})   // Network address
	f.Add([]byte{10, 42, 1, 255}) // Broadcast address
	f.Add([]byte{192, 168, 1, 1}) // Outside subnet

	f.Fuzz(func(t *testing.T, ipBytes []byte) {
		// Only test with 4-byte inputs (IPv4)
		if len(ipBytes) != 4 {
			return
		}

		var arr [4]byte
		copy(arr[:], ipBytes)
		ip := netip.AddrFrom4(arr)

		// ValidateTunnelIP should never panic
		err := ValidateTunnelIP(ip, TunnelSubnet)

		// If validation succeeds, IP must be in subnet and not reserved
		if err == nil {
			if !TunnelSubnet.Contains(ip) {
				t.Errorf("ValidateTunnelIP succeeded for IP outside subnet: %v", ip)
			}
			if arr[3] == 0 {
				t.Error("ValidateTunnelIP succeeded for network address")
			}
			if arr[3] == 255 {
				t.Error("ValidateTunnelIP succeeded for broadcast address")
			}
		}
	})
}
