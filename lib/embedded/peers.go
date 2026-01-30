package embedded

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/go-i2p/wireguard/lib/identity"
	"github.com/go-i2p/wireguard/lib/rpc"
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
// convertRPCPeer converts an RPC peer to a PeerInfo struct.
func convertRPCPeer(p struct {
	NodeID      string
	TunnelIP    string
	State       string
	LastSeen    string
	ConnectedAt string
	Latency     string
},
) PeerInfo {
	var tunnelIP netip.Addr
	if p.TunnelIP != "" {
		tunnelIP, _ = netip.ParseAddr(p.TunnelIP)
	}
	lastSeen, connectedAt := parsePeerTimestamps(p.LastSeen, p.ConnectedAt)
	var latency time.Duration
	if p.Latency != "" {
		latency, _ = time.ParseDuration(p.Latency)
	}
	return PeerInfo{
		NodeID:      p.NodeID,
		TunnelIP:    tunnelIP,
		State:       p.State,
		LastSeen:    lastSeen,
		ConnectedAt: connectedAt,
		Latency:     latency,
	}
}

func (v *VPN) Peers() []PeerInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	rpcPeers := v.node.ListPeers()
	if len(rpcPeers) == 0 {
		return nil
	}

	result := make([]PeerInfo, len(rpcPeers))
	for i, p := range rpcPeers {
		result[i] = convertRPCPeer(struct {
			NodeID      string
			TunnelIP    string
			State       string
			LastSeen    string
			ConnectedAt string
			Latency     string
		}{p.NodeID, p.TunnelIP, p.State, p.LastSeen, p.ConnectedAt, p.Latency})
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
// parsePeerTimestamps parses the timestamp strings from an RPC peer.
func parsePeerTimestamps(lastSeenStr, connectedAtStr string) (lastSeen, connectedAt time.Time) {
	if lastSeenStr != "" {
		lastSeen, _ = time.Parse(time.RFC3339, lastSeenStr)
	}
	if connectedAtStr != "" {
		connectedAt, _ = time.Parse(time.RFC3339, connectedAtStr)
	}
	return
}

func (v *VPN) GetPeer(nodeID string) *PeerInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		return nil
	}

	return v.findPeerByNodeID(nodeID)
}

// findPeerByNodeID searches for a peer by node ID and returns its info.
func (v *VPN) findPeerByNodeID(nodeID string) *PeerInfo {
	rpcPeers := v.node.ListPeers()
	for _, p := range rpcPeers {
		if p.NodeID == nodeID {
			return v.convertRPCPeerInfoToInfo(p)
		}
	}
	return nil
}

// convertRPCPeerInfoToInfo converts an rpc.PeerInfo struct to a PeerInfo pointer.
func (v *VPN) convertRPCPeerInfoToInfo(p rpc.PeerInfo) *PeerInfo {
	var tunnelIP netip.Addr
	if p.TunnelIP != "" {
		tunnelIP, _ = netip.ParseAddr(p.TunnelIP)
	}
	lastSeen, connectedAt := parsePeerTimestamps(p.LastSeen, p.ConnectedAt)
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

// CreateInvite generates an invite code for a new peer to join the network.
// The invite expires after the specified duration and can be used up to maxUses times.
// Use maxUses=0 for unlimited uses (not recommended for security).
func (v *VPN) CreateInvite(expiry time.Duration, maxUses int) (string, error) {
	log.WithFields(map[string]interface{}{
		"expiry":   expiry.String(),
		"max_uses": maxUses,
	}).Debug("Creating invite")

	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		log.Warn("CreateInvite called while VPN not running")
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
		log.WithError(err).Error("Failed to create invite")
		return "", err
	}

	log.Debug("Invite created successfully")
	v.emitter.emitSimple(EventInviteCreated, "Created invite code")
	return result.InviteCode, nil
}

// AcceptInvite connects to a network using an invite code.
// This will establish a connection to the inviting peer and join their mesh network.
func (v *VPN) AcceptInvite(ctx context.Context, inviteCode string) error {
	log.Debug("Accepting invite code")

	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		log.Warn("AcceptInvite called while VPN not running")
		return errors.New("VPN is not running")
	}

	if inviteCode == "" {
		log.Warn("AcceptInvite called with empty invite code")
		return errors.New("invite code is required")
	}

	// Delegate to core.Node's AcceptInvite
	_, err := node.AcceptInvite(ctx, inviteCode)
	if err != nil {
		log.WithError(err).Error("Failed to accept invite")
		return err
	}

	log.Debug("Invite accepted successfully, joined network")
	v.emitter.emitSimple(EventInviteAccepted, "Successfully joined network")
	return nil
}

// Routes returns a list of all known routes.
func (v *VPN) Routes() []RouteInfo {
	log.Debug("Retrieving routes")

	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		log.Debug("Routes called while VPN not running, returning nil")
		return nil
	}

	// Delegate to core.Node's ListRoutes
	rpcRoutes := v.node.ListRoutes()
	if len(rpcRoutes) == 0 {
		log.Debug("No routes found")
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
	log.Debug("Listing invites")

	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.state != StateRunning || v.node == nil {
		log.Debug("ListInvites called while VPN not running")
		return nil
	}

	store := v.node.InviteStore()
	if store == nil {
		log.Debug("Invite store not available")
		return nil
	}

	invites := store.ListGenerated()
	if len(invites) == 0 {
		log.Debug("No invites found")
		return nil
	}

	log.WithField("count", len(invites)).Debug("Found invites")
	return v.filterAndConvertInvites(invites)
}

// filterAndConvertInvites filters out expired invites and converts valid ones to InviteInfo.
func (v *VPN) filterAndConvertInvites(invites []*identity.Invite) []InviteInfo {
	result := make([]InviteInfo, 0, len(invites))
	for _, inv := range invites {
		if inv.IsExpired() {
			continue
		}
		info := v.convertInviteToInfo(inv)
		if info != nil {
			result = append(result, *info)
		}
	}
	return result
}

// convertInviteToInfo converts an identity.Invite to an InviteInfo struct.
func (v *VPN) convertInviteToInfo(inv *identity.Invite) *InviteInfo {
	code, err := inv.Encode()
	if err != nil {
		return nil
	}
	return &InviteInfo{
		Code:          code,
		ExpiresAt:     inv.ExpiresAt,
		MaxUses:       inv.MaxUses,
		UsesRemaining: inv.MaxUses - inv.UsedCount,
		NetworkID:     inv.NetworkID,
	}
}

// RevokeInvite revokes an invite code so it can no longer be used.
func (v *VPN) RevokeInvite(inviteCode string) error {
	log.Debug("Revoking invite")

	if err := v.validateVPNRunning(); err != nil {
		log.Warn("RevokeInvite called while VPN not running")
		return err
	}

	if inviteCode == "" {
		log.Warn("RevokeInvite called with empty invite code")
		return errors.New("invite code is required")
	}

	store := v.node.InviteStore()
	if store == nil {
		log.Error("Invite store not available")
		return errors.New("invite store not available")
	}

	if err := v.removeInviteFromStore(store, inviteCode); err != nil {
		log.WithError(err).Error("Failed to revoke invite")
		return err
	}

	log.Debug("Invite revoked successfully")
	v.emitter.emitSimple(EventInviteRevoked, "Invite revoked")
	return nil
}

// validateVPNRunning checks if the VPN is running and returns an error if not.
func (v *VPN) validateVPNRunning() error {
	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		return errors.New("VPN is not running")
	}
	return nil
}

// removeInviteFromStore parses the invite code and removes it from the store.
func (v *VPN) removeInviteFromStore(store *identity.InviteStore, inviteCode string) error {
	invite, err := parseInviteCode(inviteCode)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%x", invite.AuthToken)

	if _, exists := store.GetGenerated(key); !exists {
		return errors.New("invite not found")
	}

	store.RemoveGenerated(key)
	if err := store.Save(); err != nil {
		return fmt.Errorf("saving invite store: %w", err)
	}

	return nil
}

// BanPeer adds a peer to the ban list.
func (v *VPN) BanPeer(nodeID, reason string, duration time.Duration) error {
	log.WithFields(map[string]interface{}{
		"node_id":  nodeID,
		"reason":   reason,
		"duration": duration.String(),
	}).Debug("Banning peer")

	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		log.Warn("BanPeer called while VPN not running")
		return errors.New("VPN is not running")
	}

	if nodeID == "" {
		log.Warn("BanPeer called with empty node ID")
		return errors.New("node ID is required")
	}

	// Delegate to core.Node's AddBan
	if err := node.AddBan(nodeID, reason, "", duration); err != nil {
		log.WithError(err).WithField("node_id", nodeID).Error("Failed to ban peer")
		return err
	}

	log.WithField("node_id", nodeID).Debug("Peer banned successfully")
	return nil
}

// UnbanPeer removes a peer from the ban list.
func (v *VPN) UnbanPeer(nodeID string) error {
	log.WithField("node_id", nodeID).Debug("Unbanning peer")

	v.mu.RLock()
	state := v.state
	node := v.node
	v.mu.RUnlock()

	if state != StateRunning || node == nil {
		log.Warn("UnbanPeer called while VPN not running")
		return errors.New("VPN is not running")
	}

	if nodeID == "" {
		log.Warn("UnbanPeer called with empty node ID")
		return errors.New("node ID is required")
	}

	// Delegate to core.Node's RemoveBan
	if !node.RemoveBan(nodeID) {
		log.WithField("node_id", nodeID).Warn("Peer not found in ban list")
		return errors.New("peer not found in ban list")
	}

	log.WithField("node_id", nodeID).Debug("Peer unbanned successfully")
	return nil
}

// parseInviteCode parses an invite code string into an Invite struct.
// This is a wrapper around identity.ParseInvite for use within the embedded package.
func parseInviteCode(code string) (*identity.Invite, error) {
	return identity.ParseInvite(code)
}
