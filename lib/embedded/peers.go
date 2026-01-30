package embedded

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/go-i2p/wireguard/lib/identity"
)

// PeerInfo contains information about a connected or known peer.
type PeerInfo struct {
	// NodeID is the unique identifier for this peer.
	NodeID string
	// TunnelIP is the peer's mesh tunnel IP address.
	TunnelIP netip.Addr
	// I2PDest is the peer's I2P destination (base32).
	I2PDest string
	// State is the peer's connection state (connected, disconnected, etc.).
	State string
	// LastSeen is when we last received data from this peer.
	LastSeen time.Time
	// ConnectedAt is when the peer connection was established.
	ConnectedAt time.Time
	// Latency is the estimated round-trip time to this peer.
	Latency time.Duration
}

// RouteInfo contains information about a mesh route.
type RouteInfo struct {
	// TunnelIP is the destination IP for this route.
	TunnelIP netip.Addr
	// NodeID is the node ID of the destination.
	NodeID string
	// ViaNodeID is the next-hop node ID (empty for direct routes).
	ViaNodeID string
	// HopCount is the number of hops to reach this destination.
	HopCount int
	// LastSeen is when this route was last verified.
	LastSeen time.Time
}

// InviteInfo contains information about an invite code.
type InviteInfo struct {
	// Code is the full invite code string.
	Code string
	// ExpiresAt is when the invite expires.
	ExpiresAt time.Time
	// MaxUses is the maximum number of times this invite can be used.
	MaxUses int
	// UsesRemaining is how many times the invite can still be used.
	UsesRemaining int
	// NetworkID is the network this invite is for.
	NetworkID string
}

// Peers returns a list of all known peers.
// This includes connected, disconnected, and stale peers.
func (v *VPN) Peers() []PeerInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	// Delegate to core.Node's ListPeers
	rpcPeers := v.node.ListPeers()
	if len(rpcPeers) == 0 {
		return nil
	}

	result := make([]PeerInfo, len(rpcPeers))
	for i, p := range rpcPeers {
		var tunnelIP netip.Addr
		if p.TunnelIP != "" {
			tunnelIP, _ = netip.ParseAddr(p.TunnelIP)
		}
		var lastSeen, connectedAt time.Time
		if p.LastSeen != "" {
			lastSeen, _ = time.Parse(time.RFC3339, p.LastSeen)
		}
		if p.ConnectedAt != "" {
			connectedAt, _ = time.Parse(time.RFC3339, p.ConnectedAt)
		}
		var latency time.Duration
		if p.Latency != "" {
			latency, _ = time.ParseDuration(p.Latency)
		}
		result[i] = PeerInfo{
			NodeID:      p.NodeID,
			TunnelIP:    tunnelIP,
			State:       p.State,
			LastSeen:    lastSeen,
			ConnectedAt: connectedAt,
			Latency:     latency,
		}
	}
	return result
}

// PeerCount returns the number of connected peers.
func (v *VPN) PeerCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return 0
	}

	// Delegate to core.Node's PeerCount
	return v.node.PeerCount()
}

// GetPeer returns information about a specific peer by node ID.
// Returns nil if the peer is not found.
func (v *VPN) GetPeer(nodeID string) *PeerInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	// Get all peers and find the matching one
	rpcPeers := v.node.ListPeers()
	for _, p := range rpcPeers {
		if p.NodeID == nodeID {
			var tunnelIP netip.Addr
			if p.TunnelIP != "" {
				tunnelIP, _ = netip.ParseAddr(p.TunnelIP)
			}
			var lastSeen, connectedAt time.Time
			if p.LastSeen != "" {
				lastSeen, _ = time.Parse(time.RFC3339, p.LastSeen)
			}
			if p.ConnectedAt != "" {
				connectedAt, _ = time.Parse(time.RFC3339, p.ConnectedAt)
			}
			var latency time.Duration
			if p.Latency != "" {
				latency, _ = time.ParseDuration(p.Latency)
			}
			return &PeerInfo{
				NodeID:      p.NodeID,
				TunnelIP:    tunnelIP,
				State:       p.State,
				LastSeen:    lastSeen,
				ConnectedAt: connectedAt,
				Latency:     latency,
			}
		}
	}
	return nil
}

// CreateInvite generates an invite code for a new peer to join the network.
// The invite expires after the specified duration and can be used up to maxUses times.
// Use maxUses=0 for unlimited uses (not recommended for security).
func (v *VPN) CreateInvite(expiry time.Duration, maxUses int) (string, error) {
	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		return "", errors.New("VPN is not running")
	}

	if expiry <= 0 {
		expiry = 24 * time.Hour
	}
	// Note: maxUses=0 means unlimited uses (passed through intentionally)
	// Only convert negative values to the default of 1
	if maxUses < 0 {
		maxUses = 1
	}

	// Delegate to core.Node's CreateInvite
	result, err := node.CreateInvite(expiry, maxUses)
	if err != nil {
		return "", err
	}

	v.emitter.emitSimple(EventInviteCreated, "Created invite code")
	return result.InviteCode, nil
}

// AcceptInvite connects to a network using an invite code.
// This will establish a connection to the inviting peer and join their mesh network.
func (v *VPN) AcceptInvite(ctx context.Context, inviteCode string) error {
	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		return errors.New("VPN is not running")
	}

	if inviteCode == "" {
		return errors.New("invite code is required")
	}

	// Delegate to core.Node's AcceptInvite
	_, err := node.AcceptInvite(ctx, inviteCode)
	if err != nil {
		return err
	}

	v.emitter.emitSimple(EventInviteAccepted, "Successfully joined network")
	return nil
}

// Routes returns a list of all known routes.
func (v *VPN) Routes() []RouteInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	// Delegate to core.Node's ListRoutes
	rpcRoutes := v.node.ListRoutes()
	if len(rpcRoutes) == 0 {
		return nil
	}

	result := make([]RouteInfo, len(rpcRoutes))
	for i, r := range rpcRoutes {
		var tunnelIP netip.Addr
		if r.TunnelIP != "" {
			tunnelIP, _ = netip.ParseAddr(r.TunnelIP)
		}
		var lastSeen time.Time
		if r.LastSeen != "" {
			lastSeen, _ = time.Parse(time.RFC3339, r.LastSeen)
		}
		result[i] = RouteInfo{
			TunnelIP:  tunnelIP,
			NodeID:    r.NodeID,
			ViaNodeID: r.ViaNodeID,
			HopCount:  r.HopCount,
			LastSeen:  lastSeen,
		}
	}
	return result
}

// RouteCount returns the number of known routes.
func (v *VPN) RouteCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return 0
	}

	// Delegate to core.Node's ListRoutes
	return len(v.node.ListRoutes())
}

// ListInvites returns a list of active invites created by this node.
func (v *VPN) ListInvites() []InviteInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	// Get the invite store from the node
	store := v.node.InviteStore()
	if store == nil {
		return nil
	}

	invites := store.ListGenerated()
	if len(invites) == 0 {
		return nil
	}

	result := make([]InviteInfo, 0, len(invites))
	for _, inv := range invites {
		// Skip expired invites
		if inv.IsExpired() {
			continue
		}
		code, err := inv.Encode()
		if err != nil {
			continue
		}
		result = append(result, InviteInfo{
			Code:          code,
			ExpiresAt:     inv.ExpiresAt,
			MaxUses:       inv.MaxUses,
			UsesRemaining: inv.MaxUses - inv.UsedCount,
			NetworkID:     inv.NetworkID,
		})
	}
	return result
}

// RevokeInvite revokes an invite code so it can no longer be used.
func (v *VPN) RevokeInvite(inviteCode string) error {
	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		return errors.New("VPN is not running")
	}

	if inviteCode == "" {
		return errors.New("invite code is required")
	}

	// Get the invite store
	store := node.InviteStore()
	if store == nil {
		return errors.New("invite store not available")
	}

	// Parse the invite to get the auth token (used as key)
	invite, err := parseInviteCode(inviteCode)
	if err != nil {
		return err
	}

	// The key is the hex-encoded auth token
	key := fmt.Sprintf("%x", invite.AuthToken)

	// Check if the invite exists
	if _, exists := store.GetGenerated(key); !exists {
		return errors.New("invite not found")
	}

	// Remove the invite
	store.RemoveGenerated(key)
	if err := store.Save(); err != nil {
		return fmt.Errorf("saving invite store: %w", err)
	}

	v.emitter.emitSimple(EventInviteRevoked, "Invite revoked")
	return nil
}

// BanPeer adds a peer to the ban list.
func (v *VPN) BanPeer(nodeID, reason string, duration time.Duration) error {
	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		return errors.New("VPN is not running")
	}

	if nodeID == "" {
		return errors.New("node ID is required")
	}

	// Delegate to core.Node's AddBan
	return node.AddBan(nodeID, reason, "", duration)
}

// UnbanPeer removes a peer from the ban list.
func (v *VPN) UnbanPeer(nodeID string) error {
	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		return errors.New("VPN is not running")
	}

	if nodeID == "" {
		return errors.New("node ID is required")
	}

	// Delegate to core.Node's RemoveBan
	if !node.RemoveBan(nodeID) {
		return errors.New("peer not found in ban list")
	}
	return nil
}

// parseInviteCode parses an invite code string into an Invite struct.
// This is a wrapper around identity.ParseInvite for use within the embedded package.
func parseInviteCode(code string) (*identity.Invite, error) {
	return identity.ParseInvite(code)
}
