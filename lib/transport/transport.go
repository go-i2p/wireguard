// Package transport provides a mesh-aware wrapper around i2pbind
// for WireGuard-over-I2P transport. It manages the I2P session,
// tracks connected peers, and exposes the local I2P address.
package transport

import (
	"sync"

	apperrors "github.com/go-i2p/wireguard/lib/errors"

	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/wireguard/i2pbind"
	"golang.zx2c4.com/wireguard/conn"
)

// PeerState represents the connection state of a peer.
type PeerState int

const (
	// PeerStateUnknown is the initial state.
	PeerStateUnknown PeerState = iota
	// PeerStateConnecting means we're attempting to connect.
	PeerStateConnecting
	// PeerStateConnected means the peer is actively connected.
	PeerStateConnected
	// PeerStateStale means the peer hasn't been seen recently.
	PeerStateStale
	// PeerStateDisconnected means the peer is not connected.
	PeerStateDisconnected
)

func (s PeerState) String() string {
	switch s {
	case PeerStateUnknown:
		return "unknown"
	case PeerStateConnecting:
		return "connecting"
	case PeerStateConnected:
		return "connected"
	case PeerStateStale:
		return "stale"
	case PeerStateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// TrackedPeer holds information about a connected peer.
type TrackedPeer struct {
	// I2PDest is the peer's I2P destination (base32)
	I2PDest string

	// WGPublicKey is the peer's WireGuard public key (base64)
	WGPublicKey string

	// State is the current connection state
	State PeerState

	// Endpoint is the WireGuard endpoint for this peer
	Endpoint conn.Endpoint
}

// Transport wraps i2pbind.I2PBind with mesh-aware functionality.
// It manages the I2P session lifecycle and tracks connected peers.
type Transport struct {
	mu sync.RWMutex

	// Configuration
	name    string
	samAddr string
	options []string

	// Underlying bind
	bind *i2pbind.I2PBind

	// State
	isOpen       bool
	localI2PDest string

	// Connected peers indexed by I2P destination
	peers map[string]*TrackedPeer
}

// NewTransport creates a new mesh transport.
func NewTransport(name, samAddr string, options []string) *Transport {
	return &Transport{
		name:    name,
		samAddr: samAddr,
		options: options,
		peers:   make(map[string]*TrackedPeer),
	}
}

// Open initializes the I2P transport layer.
// The Transport owns the bind - use Bind() to access it for WireGuard device creation.
// Call Close() on the Transport to clean up; do not close the bind directly.
func (t *Transport) Open() error {
	log.WithField("name", t.name).Debug("opening transport")
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isOpen {
		log.Warn("transport already open")
		return apperrors.ErrTransportAlreadyOpen
	}

	// Create the underlying I2P bind
	t.bind = i2pbind.NewI2PBindWithOptions(t.name, t.samAddr, t.options)

	// Open the bind to start the I2P session
	_, _, err := t.bind.Open(0)
	if err != nil {
		log.WithError(err).Error("failed to open I2P bind")
		return err
	}

	// Get our local I2P address
	localAddr, err := t.bind.LocalAddress()
	if err != nil {
		log.WithError(err).Error("failed to get local I2P address")
		t.bind.Close()
		return err
	}

	t.localI2PDest = localAddr
	t.isOpen = true
	log.WithField("localAddress", localAddr).Debug("transport opened successfully")

	return nil
}

// SetMeshHandler sets the callback for handling mesh protocol messages.
// Non-WireGuard messages (gossip, handshake, etc.) will be routed to this handler.
// For best results, call this before the transport starts receiving (i.e., before
// the WireGuard device is created). If called after messages have been received,
// a warning will be logged by the underlying bind.
func (t *Transport) SetMeshHandler(handler i2pbind.MeshMessageHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.bind != nil {
		t.bind.SetMeshHandler(handler)
	}
}

// Bind returns the underlying WireGuard Bind interface.
// The Transport retains ownership - do not close the bind directly.
// Returns nil if the transport is not open.
func (t *Transport) Bind() conn.Bind {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.bind
}

// Close shuts down the I2P transport.
func (t *Transport) Close() error {
	log.Debug("closing transport")
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isOpen {
		log.Debug("transport already closed")
		return nil
	}

	t.isOpen = false
	t.localI2PDest = ""
	t.peers = make(map[string]*TrackedPeer)

	if t.bind != nil {
		err := t.bind.Close()
		t.bind = nil
		if err != nil {
			log.WithError(err).Warn("error closing bind")
		}
		log.Debug("transport closed")
		return err
	}

	return nil
}

// LocalAddress returns the local I2P destination address (base32).
// Returns empty string if the transport is not open.
func (t *Transport) LocalAddress() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.localI2PDest
}

// LocalDestination returns the full I2P destination.
// Returns an error if the transport is not open.
func (t *Transport) LocalDestination() (i2pkeys.I2PAddr, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.isOpen || t.bind == nil {
		return "", apperrors.ErrTransportNotOpen
	}

	return t.bind.LocalDestination()
}

// IsOpen returns true if the transport is open.
func (t *Transport) IsOpen() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isOpen
}

// AddPeer registers a new peer with the transport.
func (t *Transport) AddPeer(i2pDest, wgPubKey string) error {
	log.WithField("i2pDest", i2pDest).Debug("adding peer to transport")
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isOpen {
		log.Warn("cannot add peer: transport not open")
		return apperrors.ErrTransportNotOpen
	}

	t.peers[i2pDest] = &TrackedPeer{
		I2PDest:     i2pDest,
		WGPublicKey: wgPubKey,
		State:       PeerStateConnecting,
	}

	return nil
}

// RemovePeer removes a peer from tracking.
func (t *Transport) RemovePeer(i2pDest string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.peers, i2pDest)
}

// GetPeer returns information about a tracked peer.
func (t *Transport) GetPeer(i2pDest string) (*TrackedPeer, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	peer, ok := t.peers[i2pDest]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent races
	copy := *peer
	return &copy, true
}

// UpdatePeerState updates the state of a tracked peer.
func (t *Transport) UpdatePeerState(i2pDest string, state PeerState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if peer, ok := t.peers[i2pDest]; ok {
		peer.State = state
	}
}

// SetPeerEndpoint sets the WireGuard endpoint for a peer.
func (t *Transport) SetPeerEndpoint(i2pDest string, ep conn.Endpoint) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if peer, ok := t.peers[i2pDest]; ok {
		peer.Endpoint = ep
		peer.State = PeerStateConnected
	}
}

// ListPeers returns a list of all tracked peers.
func (t *Transport) ListPeers() []TrackedPeer {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]TrackedPeer, 0, len(t.peers))
	for _, peer := range t.peers {
		result = append(result, *peer)
	}
	return result
}

// PeerCount returns the number of tracked peers.
func (t *Transport) PeerCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.peers)
}

// ConnectedPeerCount returns the number of peers in connected state.
func (t *Transport) ConnectedPeerCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, peer := range t.peers {
		if peer.State == PeerStateConnected {
			count++
		}
	}
	return count
}

// ParseEndpoint parses an I2P address into a WireGuard endpoint.
func (t *Transport) ParseEndpoint(i2pDest string) (conn.Endpoint, error) {
	t.mu.RLock()
	bind := t.bind
	t.mu.RUnlock()

	if bind == nil {
		return nil, apperrors.ErrTransportNotOpen
	}

	return bind.ParseEndpoint(i2pDest)
}

// Reconnect attempts to close and reopen the I2P transport.
// The I2P identity is persistent (keys are stored by tunnel name via onramp),
// so the same I2P destination will be restored after reconnection.
// Returns the local destination address on success.
func (t *Transport) Reconnect() (string, error) {
	log.Debug("reconnecting transport")
	t.mu.Lock()
	defer t.mu.Unlock()

	// Close existing session if open
	if t.isOpen && t.bind != nil {
		log.Debug("closing existing session")
		t.bind.Close()
		t.bind = nil
		t.isOpen = false
		t.localI2PDest = ""
	}

	// Create new I2P bind
	t.bind = i2pbind.NewI2PBindWithOptions(t.name, t.samAddr, t.options)

	// Open the bind to start new I2P session
	_, _, err := t.bind.Open(0)
	if err != nil {
		log.WithError(err).Error("failed to reopen bind")
		t.bind = nil
		return "", err
	}

	// Get our new local I2P address
	localAddr, err := t.bind.LocalAddress()
	if err != nil {
		log.WithError(err).Error("failed to get local address after reconnect")
		t.bind.Close()
		t.bind = nil
		return "", err
	}

	t.localI2PDest = localAddr
	t.isOpen = true
	log.WithField("localAddress", localAddr).Debug("transport reconnected successfully")
	// Note: peers map is preserved - caller may want to re-establish connections

	return localAddr, nil
}
