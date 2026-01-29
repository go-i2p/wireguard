package embedded

import (
	"context"
	"errors"
	"net/netip"
	"time"
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

	// TODO: Once core.Node exposes peer management, delegate to it
	// return v.node.PeerManager().ListPeers()

	return []PeerInfo{}
}

// PeerCount returns the number of connected peers.
func (v *VPN) PeerCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return 0
	}

	// TODO: Once core.Node exposes peer management, delegate to it
	// return v.node.PeerManager().ConnectedCount()

	return 0
}

// GetPeer returns information about a specific peer by node ID.
// Returns nil if the peer is not found.
func (v *VPN) GetPeer(nodeID string) *PeerInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	// TODO: Once core.Node exposes peer management, delegate to it
	// peer := v.node.PeerManager().GetPeer(nodeID)
	// if peer == nil { return nil }
	// return &PeerInfo{...}

	return nil
}

// CreateInvite generates an invite code for a new peer to join the network.
// The invite expires after the specified duration and can be used up to maxUses times.
// Use maxUses=0 for unlimited uses (not recommended for security).
func (v *VPN) CreateInvite(expiry time.Duration, maxUses int) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return "", errors.New("VPN is not running")
	}

	if expiry <= 0 {
		expiry = 24 * time.Hour
	}
	if maxUses < 0 {
		maxUses = 1
	}

	// TODO: Once core.Node exposes invite management, delegate to it
	// return v.node.InviteManager().Create(expiry, maxUses)

	return "", errors.New("invite creation not yet implemented")
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

	// TODO: Once core.Node exposes invite management, delegate to it
	// err := v.node.InviteManager().Accept(ctx, inviteCode)
	// if err != nil { return err }
	// v.emitter.emitSimple(EventInviteAccepted, "Successfully joined network")
	// return nil

	return errors.New("invite acceptance not yet implemented")
}

// Routes returns a list of all known routes.
func (v *VPN) Routes() []RouteInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	// TODO: Once core.Node exposes routing table, delegate to it
	// return v.node.RoutingTable().ListRoutes()

	return []RouteInfo{}
}

// RouteCount returns the number of known routes.
func (v *VPN) RouteCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return 0
	}

	// TODO: Once core.Node exposes routing table, delegate to it
	// return v.node.RoutingTable().Count()

	return 0
}

// ListInvites returns a list of active invites created by this node.
func (v *VPN) ListInvites() []InviteInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	// TODO: Once core.Node exposes invite management, delegate to it
	// return v.node.InviteManager().List()

	return []InviteInfo{}
}

// RevokeInvite revokes an invite code so it can no longer be used.
func (v *VPN) RevokeInvite(inviteCode string) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return errors.New("VPN is not running")
	}

	if inviteCode == "" {
		return errors.New("invite code is required")
	}

	// TODO: Once core.Node exposes invite management, delegate to it
	// return v.node.InviteManager().Revoke(inviteCode)

	return errors.New("invite revocation not yet implemented")
}

// BanPeer adds a peer to the ban list.
func (v *VPN) BanPeer(nodeID, reason string, duration time.Duration) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return errors.New("VPN is not running")
	}

	if nodeID == "" {
		return errors.New("node ID is required")
	}

	// TODO: Once core.Node exposes ban list, delegate to it
	// return v.node.BanList().Add(nodeID, reason, duration)

	return errors.New("peer banning not yet implemented")
}

// UnbanPeer removes a peer from the ban list.
func (v *VPN) UnbanPeer(nodeID string) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return errors.New("VPN is not running")
	}

	if nodeID == "" {
		return errors.New("node ID is required")
	}

	// TODO: Once core.Node exposes ban list, delegate to it
	// return v.node.BanList().Remove(nodeID)

	return errors.New("peer unbanning not yet implemented")
}
