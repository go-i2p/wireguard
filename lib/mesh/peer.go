package mesh

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// PeerState represents the connection state of a mesh peer.
type PeerState int

const (
	// PeerStateUnknown is the initial state.
	PeerStateUnknown PeerState = iota
	// PeerStatePending means handshake is in progress.
	PeerStatePending
	// PeerStateConnected means handshake completed successfully.
	PeerStateConnected
	// PeerStateDisconnected means peer was connected but lost.
	PeerStateDisconnected
	// PeerStateRejected means handshake was rejected.
	PeerStateRejected
)

func (s PeerState) String() string {
	switch s {
	case PeerStateUnknown:
		return "Unknown"
	case PeerStatePending:
		return "Pending"
	case PeerStateConnected:
		return "Connected"
	case PeerStateDisconnected:
		return "Disconnected"
	case PeerStateRejected:
		return "Rejected"
	default:
		return fmt.Sprintf("PeerState(%d)", s)
	}
}

// Peer represents a known mesh network peer.
type Peer struct {
	// NodeID is the unique identifier derived from WG public key
	NodeID string
	// I2PDest is the peer's I2P destination
	I2PDest string
	// WGPublicKey is the peer's WireGuard public key
	WGPublicKey wgtypes.Key
	// TunnelIP is the peer's mesh tunnel IP
	TunnelIP netip.Addr
	// State is the current connection state
	State PeerState
	// LastSeen is when we last heard from this peer
	LastSeen time.Time
	// ConnectedAt is when the handshake completed
	ConnectedAt time.Time
	// Latency is estimated round-trip time
	Latency time.Duration
}

// PeerManager handles peer discovery, handshakes, and state.
type PeerManager struct {
	mu     sync.RWMutex
	peers  map[string]*Peer // keyed by NodeID
	logger *slog.Logger

	// Our identity for handshakes
	nodeID      string
	i2pDest     string
	wgPublicKey wgtypes.Key
	tunnelIP    netip.Addr
	networkID   string

	// Validation
	validTokens [][]byte // invite tokens that can authorize new peers

	// Callbacks
	onPeerConnected    func(*Peer)
	onPeerDisconnected func(*Peer)

	// Configuration
	maxPeers        int
	handshakeTimout time.Duration
}

// PeerManagerConfig configures the PeerManager.
type PeerManagerConfig struct {
	NodeID      string
	I2PDest     string
	WGPublicKey wgtypes.Key
	TunnelIP    netip.Addr
	NetworkID   string
	MaxPeers    int
	Logger      *slog.Logger
}

// NewPeerManager creates a new PeerManager.
func NewPeerManager(cfg PeerManagerConfig) *PeerManager {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	maxPeers := cfg.MaxPeers
	if maxPeers <= 0 {
		maxPeers = 50
	}

	return &PeerManager{
		peers:           make(map[string]*Peer),
		logger:          logger,
		nodeID:          cfg.NodeID,
		i2pDest:         cfg.I2PDest,
		wgPublicKey:     cfg.WGPublicKey,
		tunnelIP:        cfg.TunnelIP,
		networkID:       cfg.NetworkID,
		maxPeers:        maxPeers,
		handshakeTimout: 30 * time.Second,
	}
}

// AddValidToken adds an invite token that can authorize new peers.
func (pm *PeerManager) AddValidToken(token []byte) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.validTokens = append(pm.validTokens, token)
}

// SetCallbacks sets the peer state change callbacks.
func (pm *PeerManager) SetCallbacks(onConnected, onDisconnected func(*Peer)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onPeerConnected = onConnected
	pm.onPeerDisconnected = onDisconnected
}

// HandleHandshakeInit processes an incoming handshake initiation.
func (pm *PeerManager) HandleHandshakeInit(init *HandshakeInit) (*HandshakeResponse, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.logger.Info("received handshake init",
		"from_node", init.NodeID,
		"i2p_dest", truncateDest(init.I2PDest))

	// Validate network ID
	if init.NetworkID != pm.networkID {
		return pm.rejectHandshake("network ID mismatch"), nil
	}

	// Validate auth token
	if !pm.validateToken(init.AuthToken) {
		return pm.rejectHandshake("invalid auth token"), nil
	}

	// Parse WireGuard public key
	pubKey, err := wgtypes.ParseKey(init.WGPublicKey)
	if err != nil {
		return pm.rejectHandshake("invalid WG public key"), nil
	}

	// Validate tunnel IP
	claimedIP, err := netip.ParseAddr(init.TunnelIP)
	if err != nil {
		return pm.rejectHandshake("invalid tunnel IP"), nil
	}

	// Verify tunnel IP matches what we'd derive from the public key
	expectedIP := AllocateTunnelIP(pubKey)
	if claimedIP != expectedIP {
		pm.logger.Warn("tunnel IP mismatch",
			"claimed", claimedIP,
			"expected", expectedIP)
		return pm.rejectHandshake("tunnel IP does not match public key"), nil
	}

	// Check if we already know this peer
	if existing, ok := pm.peers[init.NodeID]; ok {
		if existing.State == PeerStateConnected {
			// Already connected, update last seen
			existing.LastSeen = time.Now()
			pm.logger.Info("peer already connected", "node_id", init.NodeID)
		}
	}

	// Check peer limit
	if len(pm.peers) >= pm.maxPeers {
		return pm.rejectHandshake("max peers reached"), nil
	}

	// Create or update peer
	peer := &Peer{
		NodeID:      init.NodeID,
		I2PDest:     init.I2PDest,
		WGPublicKey: pubKey,
		TunnelIP:    claimedIP,
		State:       PeerStatePending,
		LastSeen:    time.Now(),
	}
	pm.peers[init.NodeID] = peer

	// Build response
	return &HandshakeResponse{
		I2PDest:     pm.i2pDest,
		WGPublicKey: pm.wgPublicKey.String(),
		TunnelIP:    pm.tunnelIP.String(),
		NetworkID:   pm.networkID,
		NodeID:      pm.nodeID,
		Accepted:    true,
	}, nil
}

// HandleHandshakeResponse processes a handshake response.
func (pm *PeerManager) HandleHandshakeResponse(resp *HandshakeResponse) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.logger.Info("received handshake response",
		"from_node", resp.NodeID,
		"accepted", resp.Accepted)

	if !resp.Accepted {
		pm.logger.Warn("handshake rejected", "reason", resp.RejectReason)
		return fmt.Errorf("handshake rejected: %s", resp.RejectReason)
	}

	// Parse WireGuard public key
	pubKey, err := wgtypes.ParseKey(resp.WGPublicKey)
	if err != nil {
		return fmt.Errorf("invalid WG public key: %w", err)
	}

	// Parse tunnel IP
	tunnelIP, err := netip.ParseAddr(resp.TunnelIP)
	if err != nil {
		return fmt.Errorf("invalid tunnel IP: %w", err)
	}

	// Verify tunnel IP matches public key
	expectedIP := AllocateTunnelIP(pubKey)
	if tunnelIP != expectedIP {
		return errors.New("tunnel IP does not match public key")
	}

	// Create or update peer
	peer := &Peer{
		NodeID:      resp.NodeID,
		I2PDest:     resp.I2PDest,
		WGPublicKey: pubKey,
		TunnelIP:    tunnelIP,
		State:       PeerStatePending,
		LastSeen:    time.Now(),
	}
	pm.peers[resp.NodeID] = peer

	return nil
}

// HandleHandshakeComplete marks a handshake as complete.
func (pm *PeerManager) HandleHandshakeComplete(complete *HandshakeComplete) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[complete.NodeID]
	if !ok {
		return errors.New("unknown peer")
	}

	if !complete.Success {
		peer.State = PeerStateRejected
		return errors.New("handshake failed")
	}

	peer.State = PeerStateConnected
	peer.ConnectedAt = time.Now()
	peer.LastSeen = time.Now()

	pm.logger.Info("peer connected",
		"node_id", peer.NodeID,
		"tunnel_ip", peer.TunnelIP)

	// Trigger callback
	if pm.onPeerConnected != nil {
		go pm.onPeerConnected(peer)
	}

	return nil
}

// CreateHandshakeInit creates a handshake initiation message.
func (pm *PeerManager) CreateHandshakeInit(authToken []byte) *HandshakeInit {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return &HandshakeInit{
		I2PDest:     pm.i2pDest,
		WGPublicKey: pm.wgPublicKey.String(),
		TunnelIP:    pm.tunnelIP.String(),
		NetworkID:   pm.networkID,
		AuthToken:   authToken,
		NodeID:      pm.nodeID,
	}
}

// CreateHandshakeComplete creates a handshake completion message.
func (pm *PeerManager) CreateHandshakeComplete(success bool) *HandshakeComplete {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return &HandshakeComplete{
		NodeID:  pm.nodeID,
		Success: success,
	}
}

// GetPeer returns a peer by NodeID.
func (pm *PeerManager) GetPeer(nodeID string) (*Peer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peer, ok := pm.peers[nodeID]
	if !ok {
		return nil, false
	}
	// Return a copy
	peerCopy := *peer
	return &peerCopy, true
}

// GetPeerByIP returns a peer by tunnel IP.
func (pm *PeerManager) GetPeerByIP(ip netip.Addr) (*Peer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, peer := range pm.peers {
		if peer.TunnelIP == ip {
			peerCopy := *peer
			return &peerCopy, true
		}
	}
	return nil, false
}

// ListPeers returns all known peers.
func (pm *PeerManager) ListPeers() []*Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*Peer, 0, len(pm.peers))
	for _, peer := range pm.peers {
		peerCopy := *peer
		result = append(result, &peerCopy)
	}
	return result
}

// ConnectedPeers returns only connected peers.
func (pm *PeerManager) ConnectedPeers() []*Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []*Peer
	for _, peer := range pm.peers {
		if peer.State == PeerStateConnected {
			peerCopy := *peer
			result = append(result, &peerCopy)
		}
	}
	return result
}

// RemovePeer removes a peer from the manager.
func (pm *PeerManager) RemovePeer(nodeID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[nodeID]
	if !ok {
		return false
	}

	delete(pm.peers, nodeID)

	// Trigger callback if was connected
	if peer.State == PeerStateConnected && pm.onPeerDisconnected != nil {
		go pm.onPeerDisconnected(peer)
	}

	return true
}

// UpdatePeerSeen updates the last seen time for a peer.
func (pm *PeerManager) UpdatePeerSeen(nodeID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer, ok := pm.peers[nodeID]; ok {
		peer.LastSeen = time.Now()
	}
}

// PruneStale removes peers that haven't been seen recently.
func (pm *PeerManager) PruneStale(maxAge time.Duration) int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	var pruned int

	for nodeID, peer := range pm.peers {
		if peer.LastSeen.Before(cutoff) {
			delete(pm.peers, nodeID)
			pruned++

			if peer.State == PeerStateConnected && pm.onPeerDisconnected != nil {
				go pm.onPeerDisconnected(peer)
			}
		}
	}

	return pruned
}

// RunHeartbeat periodically prunes stale peers.
func (pm *PeerManager) RunHeartbeat(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if pruned := pm.PruneStale(maxAge); pruned > 0 {
				pm.logger.Info("pruned stale peers", "count", pruned)
			}
		}
	}
}

// PeerCount returns the number of known peers.
func (pm *PeerManager) PeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// ConnectedCount returns the number of connected peers.
func (pm *PeerManager) ConnectedCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var count int
	for _, peer := range pm.peers {
		if peer.State == PeerStateConnected {
			count++
		}
	}
	return count
}

func (pm *PeerManager) rejectHandshake(reason string) *HandshakeResponse {
	return &HandshakeResponse{
		I2PDest:      pm.i2pDest,
		WGPublicKey:  pm.wgPublicKey.String(),
		TunnelIP:     pm.tunnelIP.String(),
		NetworkID:    pm.networkID,
		NodeID:       pm.nodeID,
		Accepted:     false,
		RejectReason: reason,
	}
}

func (pm *PeerManager) validateToken(token []byte) bool {
	for _, valid := range pm.validTokens {
		if subtle.ConstantTimeCompare(token, valid) == 1 {
			return true
		}
	}
	return false
}

func truncateDest(dest string) string {
	if len(dest) > 20 {
		return dest[:20] + "..."
	}
	return dest
}
