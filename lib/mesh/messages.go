// Package mesh provides the gossip protocol, routing table, and peer management
// for the i2plan mesh network. It handles peer discovery, state synchronization,
// and message propagation across the decentralized network.
package mesh

import (
	"crypto/sha256"
	"encoding/binary"
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

// DecodeMessage parses a Message envelope.
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshaling message: %w", err)
	}
	return &msg, nil
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

	// Get the base address
	base := subnet.Addr().As4()

	// Calculate how many bits are for the host
	hostBits := 32 - subnet.Bits()
	if hostBits <= 0 {
		return subnet.Addr()
	}

	// Use hash bytes for host portion
	// For a /16, we have 16 host bits = 2 bytes
	hostBytes := (hostBits + 7) / 8

	// Create the result address
	result := base

	// Apply hash to host portion
	for i := 0; i < hostBytes && i < len(hash); i++ {
		byteIdx := 4 - hostBytes + i
		if byteIdx >= 0 && byteIdx < 4 {
			result[byteIdx] = hash[i]
		}
	}

	// Ensure we don't get .0 or .255 for the last octet (network/broadcast)
	if result[3] == 0 {
		result[3] = 1
	} else if result[3] == 255 {
		result[3] = 254
	}

	// Keep the network portion from the original subnet
	maskBytes := subnet.Bits() / 8
	for i := 0; i < maskBytes; i++ {
		result[i] = base[i]
	}

	// Handle partial byte masking
	if subnet.Bits()%8 != 0 {
		maskBits := subnet.Bits() % 8
		mask := byte(0xFF << (8 - maskBits))
		idx := maskBytes
		result[idx] = (base[idx] & mask) | (result[idx] & ^mask)
	}

	return netip.AddrFrom4(result)
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
