// Package transport provides a mesh-aware wrapper around i2pbind.
// This file implements the MessageSender interface for gossip protocol.
package transport

import (
	"errors"
	"sync"

	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/wireguard/i2pbind"
)

// Sender implements the mesh.MessageSender interface using the I2P transport.
// It enables the gossip protocol to send messages to peers via I2P datagrams.
type Sender struct {
	mu sync.RWMutex

	// transport is the underlying I2P transport
	transport *Transport

	// peerDestinations maps nodeID -> I2P destination for sending
	peerDestinations map[string]string
}

// NewSender creates a new Sender for the given transport.
// The transport must be Open before using the sender.
func NewSender(t *Transport) *Sender {
	return &Sender{
		transport:        t,
		peerDestinations: make(map[string]string),
	}
}

// RegisterPeer maps a nodeID to an I2P destination for sending.
// This must be called before SendTo can reach the peer.
func (s *Sender) RegisterPeer(nodeID, i2pDest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerDestinations[nodeID] = i2pDest
}

// UnregisterPeer removes the nodeID to I2P destination mapping.
func (s *Sender) UnregisterPeer(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peerDestinations, nodeID)
}

// GetPeerDestination returns the I2P destination for a nodeID, if registered.
func (s *Sender) GetPeerDestination(nodeID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dest, ok := s.peerDestinations[nodeID]
	return dest, ok
}

// SendTo sends a message to a specific peer by node ID.
// The peer must be registered with RegisterPeer first.
func (s *Sender) SendTo(nodeID string, data []byte) error {
	s.mu.RLock()
	i2pDest, ok := s.peerDestinations[nodeID]
	t := s.transport
	s.mu.RUnlock()

	if !ok {
		return errors.New("peer not registered for sending")
	}

	return s.sendToDest(t, i2pDest, data)
}

// SendToDest sends a message directly to an I2P destination.
// This is useful for initial handshakes before a peer is registered.
func (s *Sender) SendToDest(i2pDest string, data []byte) error {
	s.mu.RLock()
	t := s.transport
	s.mu.RUnlock()

	return s.sendToDest(t, i2pDest, data)
}

// sendToDest is the internal send implementation.
func (s *Sender) sendToDest(t *Transport, i2pDest string, data []byte) error {
	if t == nil || !t.IsOpen() {
		return errors.New("transport not open")
	}

	// Get the underlying bind
	bind := t.Bind()
	if bind == nil {
		return errors.New("transport bind not available")
	}

	// Cast to I2PBind to access Send
	i2pBind, ok := bind.(*i2pbind.I2PBind)
	if !ok {
		return errors.New("transport is not I2P bind")
	}

	// Parse the destination to an endpoint
	endpoint, err := i2pBind.ParseEndpoint(i2pDest)
	if err != nil {
		return err
	}

	// Send the data
	return i2pBind.Send([][]byte{data}, endpoint)
}

// Broadcast sends a message to all registered peers.
// Returns the first error encountered, but attempts to send to all peers.
func (s *Sender) Broadcast(data []byte) error {
	s.mu.RLock()
	destinations := make([]string, 0, len(s.peerDestinations))
	for _, dest := range s.peerDestinations {
		destinations = append(destinations, dest)
	}
	t := s.transport
	s.mu.RUnlock()

	if len(destinations) == 0 {
		return nil // No peers to broadcast to
	}

	var firstErr error
	for _, dest := range destinations {
		if err := s.sendToDest(t, dest, data); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// LocalDestination returns our I2P destination for peer exchange.
func (s *Sender) LocalDestination() (i2pkeys.I2PAddr, error) {
	s.mu.RLock()
	t := s.transport
	s.mu.RUnlock()

	if t == nil {
		return "", errors.New("transport not available")
	}
	return t.LocalDestination()
}

// PeerCount returns the number of registered peers.
func (s *Sender) PeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.peerDestinations)
}
