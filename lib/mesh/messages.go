// Package mesh provides the gossip protocol, routing table, and peer management
// for the i2plan mesh network. It handles peer discovery, state synchronization,
// and message propagation across the decentralized network.
package mesh

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// MessageType identifies the type of mesh protocol message.
type MessageType uint8

const (
	// MsgHandshakeInit initiates a peer handshake.
	MsgHandshakeInit MessageType = iota + 1
	// MsgHandshakeResponse responds to a handshake initiation.
	MsgHandshakeResponse
	// MsgHandshakeComplete confirms handshake success.
	MsgHandshakeComplete
	// MsgPeerAnnounce announces presence to the network.
	MsgPeerAnnounce
	// MsgPeerList shares known peers.
	MsgPeerList
	// MsgRouteUpdate shares routing information.
	MsgRouteUpdate
	// MsgPeerLeave signals departure from network.
	MsgPeerLeave
)

func (m MessageType) String() string {
	switch m {
	case MsgHandshakeInit:
		return "HandshakeInit"
	case MsgHandshakeResponse:
		return "HandshakeResponse"
	case MsgHandshakeComplete:
		return "HandshakeComplete"
	case MsgPeerAnnounce:
		return "PeerAnnounce"
	case MsgPeerList:
		return "PeerList"
	case MsgRouteUpdate:
		return "RouteUpdate"
	case MsgPeerLeave:
		return "PeerLeave"
	default:
		return fmt.Sprintf("Unknown(%d)", m)
	}
}

// Message is the envelope for all mesh protocol messages.
type Message struct {
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	// SenderID is the node ID of the message sender
	SenderID string `json:"sender_id,omitempty"`
	// VerifyingKey is the sender's Ed25519 public key (hex-encoded)
	VerifyingKey string `json:"verifying_key,omitempty"`
	// Signature is the Ed25519 signature of the message (hex-encoded)
	Signature string `json:"signature,omitempty"`
}

// HandshakeInit is sent by the joining node to initiate connection.
type HandshakeInit struct {
	// I2PDest is the sender's I2P destination (base32)
	I2PDest string `json:"i2p_dest"`
	// WGPublicKey is the sender's WireGuard public key (base64)
	WGPublicKey string `json:"wg_public_key"`
	// TunnelIP is the sender's claimed tunnel IP
	TunnelIP string `json:"tunnel_ip"`
	// NetworkID is the network being joined
	NetworkID string `json:"network_id"`
	// AuthToken is from the invite code (for validation)
	AuthToken []byte `json:"auth_token"`
	// NodeID is derived from WG public key
	NodeID string `json:"node_id"`
}

// HandshakeResponse is sent back by the existing node.
type HandshakeResponse struct {
	// I2PDest is the responder's I2P destination
	I2PDest string `json:"i2p_dest"`
	// WGPublicKey is the responder's WireGuard public key
	WGPublicKey string `json:"wg_public_key"`
	// TunnelIP is the responder's tunnel IP
	TunnelIP string `json:"tunnel_ip"`
	// NetworkID confirms the network
	NetworkID string `json:"network_id"`
	// NodeID is derived from WG public key
	NodeID string `json:"node_id"`
	// Accepted indicates if the handshake is accepted
	Accepted bool `json:"accepted"`
	// RejectReason is set if Accepted is false
	RejectReason string `json:"reject_reason,omitempty"`
}

// HandshakeComplete confirms the tunnel is established.
type HandshakeComplete struct {
	// NodeID of the sender
	NodeID string `json:"node_id"`
	// Success indicates tunnel establishment succeeded
	Success bool `json:"success"`
}

// PeerAnnounce is a heartbeat message announcing presence.
type PeerAnnounce struct {
	// I2PDest is the sender's I2P destination
	I2PDest string `json:"i2p_dest"`
	// WGPublicKey is the sender's WireGuard public key
	WGPublicKey string `json:"wg_public_key"`
	// TunnelIP is the sender's tunnel IP
	TunnelIP string `json:"tunnel_ip"`
	// NetworkID identifies the network
	NetworkID string `json:"network_id"`
	// NodeID is derived from WG public key
	NodeID string `json:"node_id"`
	// PeerCount is how many peers this node knows
	PeerCount int `json:"peer_count"`
}

// MessageSigner provides signing capability for messages.
type MessageSigner interface {
	// NodeID returns the signer's node ID.
	NodeID() string
	// VerifyingKeyHex returns the Ed25519 public key as hex.
	VerifyingKeyHex() string
	// Sign signs data with the Ed25519 private key.
	Sign(data []byte) []byte
}

// EncodeMessage creates a Message envelope with the given payload.
func EncodeMessage(msgType MessageType, payload interface{}) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	msg := Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Payload:   payloadBytes,
	}

	return json.Marshal(msg)
}

// EncodeSignedMessage creates a signed Message envelope.
// The signature covers type, timestamp, and payload.
func EncodeSignedMessage(msgType MessageType, payload interface{}, signer MessageSigner) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	msg := Message{
		Type:         msgType,
		Timestamp:    time.Now(),
		Payload:      payloadBytes,
		SenderID:     signer.NodeID(),
		VerifyingKey: signer.VerifyingKeyHex(),
	}

	// Sign the message content (type + timestamp + payload)
	signableData := msg.signableBytes()
	signature := signer.Sign(signableData)
	msg.Signature = hex.EncodeToString(signature)

	return json.Marshal(msg)
}

// signableBytes returns the bytes that should be signed.
func (m *Message) signableBytes() []byte {
	// Create a deterministic byte representation for signing
	h := sha256.New()
	binary.Write(h, binary.BigEndian, uint8(m.Type))
	binary.Write(h, binary.BigEndian, m.Timestamp.UnixNano())
	h.Write(m.Payload)
	return h.Sum(nil)
}

// VerifySignature checks if the message signature is valid.
// Returns true if the signature is valid, false otherwise.
// Returns true for unsigned messages (backward compatibility).
func (m *Message) VerifySignature() bool {
	// If no signature, treat as valid (backward compatibility)
	if m.Signature == "" || m.VerifyingKey == "" {
		return true
	}

	// Decode signature
	signature, err := hex.DecodeString(m.Signature)
	if err != nil {
		return false
	}

	// Decode verifying key
	verifyingKey, err := hex.DecodeString(m.VerifyingKey)
	if err != nil || len(verifyingKey) != 32 {
		return false
	}

	// Verify the signature
	signableData := m.signableBytes()
	return ed25519.Verify(verifyingKey, signableData, signature)
}

// IsSigned returns true if the message has a signature.
func (m *Message) IsSigned() bool {
	return m.Signature != "" && m.VerifyingKey != ""
}

// DecodeMessage parses a Message envelope.
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshaling message: %w", err)
	}
	return &msg, nil
}

// DecodeAndVerifyMessage parses and verifies a signed message.
// Returns an error if the signature is present but invalid.
func DecodeAndVerifyMessage(data []byte) (*Message, error) {
	msg, err := DecodeMessage(data)
	if err != nil {
		return nil, err
	}

	// If signed, verify the signature
	if msg.IsSigned() && !msg.VerifySignature() {
		return nil, errors.New("invalid message signature")
	}

	return msg, nil
}

// DecodePayload extracts and parses the payload into the given type.
func DecodePayload[T any](msg *Message) (*T, error) {
	var payload T
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshaling payload: %w", err)
	}
	return &payload, nil
}

// TunnelSubnet is the default subnet for mesh tunnel IPs.
var TunnelSubnet = netip.MustParsePrefix("10.42.0.0/16")

// AllocateTunnelIP derives a deterministic tunnel IP from a WireGuard public key.
// The IP is derived by hashing the public key and using bytes for the host portion.
// Format: 10.42.X.Y where X.Y are derived from hash(public_key).
func AllocateTunnelIP(publicKey wgtypes.Key) netip.Addr {
	return AllocateTunnelIPWithSubnet(publicKey, TunnelSubnet)
}

// AllocateTunnelIPWithSubnet derives a tunnel IP within the given subnet.
func AllocateTunnelIPWithSubnet(publicKey wgtypes.Key, subnet netip.Prefix) netip.Addr {
	hash := sha256.Sum256(publicKey[:])
	base := subnet.Addr().As4()
	hostBits := 32 - subnet.Bits()

	if hostBits <= 0 {
		return subnet.Addr()
	}

	result := applyHashToHostPortion(base, hash, hostBits)
	result = sanitizeLastOctet(result)
	result = applyNetworkMask(result, base, subnet)

	return netip.AddrFrom4(result)
}

// applyHashToHostPortion applies hash bytes to the host portion of the address.
func applyHashToHostPortion(base [4]byte, hash [32]byte, hostBits int) [4]byte {
	result := base
	hostBytes := (hostBits + 7) / 8

	for i := 0; i < hostBytes && i < len(hash); i++ {
		byteIdx := 4 - hostBytes + i
		if byteIdx >= 0 && byteIdx < 4 {
			result[byteIdx] = hash[i]
		}
	}
	return result
}

// sanitizeLastOctet ensures the last octet is not 0 or 255 (network/broadcast).
func sanitizeLastOctet(addr [4]byte) [4]byte {
	if addr[3] == 0 {
		addr[3] = 1
	} else if addr[3] == 255 {
		addr[3] = 254
	}
	return addr
}

// applyNetworkMask preserves the network portion from the subnet.
func applyNetworkMask(result, base [4]byte, subnet netip.Prefix) [4]byte {
	maskBytes := subnet.Bits() / 8
	for i := 0; i < maskBytes; i++ {
		result[i] = base[i]
	}

	if subnet.Bits()%8 != 0 {
		maskBits := subnet.Bits() % 8
		mask := byte(0xFF << (8 - maskBits))
		idx := maskBytes
		result[idx] = (base[idx] & mask) | (result[idx] & ^mask)
	}
	return result
}

// ValidateTunnelIP checks if an IP is valid for the mesh subnet.
func ValidateTunnelIP(ip netip.Addr, subnet netip.Prefix) error {
	if !ip.IsValid() {
		return errors.New("invalid IP address")
	}
	if !ip.Is4() {
		return errors.New("only IPv4 addresses supported")
	}
	if !subnet.Contains(ip) {
		return fmt.Errorf("IP %s not in subnet %s", ip, subnet)
	}

	// Check for reserved addresses
	addr := ip.As4()
	if addr[3] == 0 {
		return errors.New("network address not allowed")
	}
	if addr[3] == 255 {
		return errors.New("broadcast address not allowed")
	}

	return nil
}

// IPCollision checks if two public keys would generate the same tunnel IP.
func IPCollision(key1, key2 wgtypes.Key, subnet netip.Prefix) bool {
	ip1 := AllocateTunnelIPWithSubnet(key1, subnet)
	ip2 := AllocateTunnelIPWithSubnet(key2, subnet)
	return ip1 == ip2
}

// HashToUint16 converts first 2 bytes of hash to uint16 for subnet host portion.
func HashToUint16(hash [32]byte) uint16 {
	return binary.BigEndian.Uint16(hash[:2])
}
