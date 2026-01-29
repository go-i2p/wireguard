package transport

import (
	"testing"
)

func TestNewSender(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	if sender == nil {
		t.Fatal("NewSender() returned nil")
	}

	if sender.transport != trans {
		t.Error("sender.transport not set correctly")
	}

	if sender.PeerCount() != 0 {
		t.Errorf("PeerCount() = %d, want 0", sender.PeerCount())
	}
}

func TestSenderPeerRegistration(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	// Register a peer
	nodeID := "test-node-123"
	i2pDest := "abcdef1234567890.b32.i2p"

	sender.RegisterPeer(nodeID, i2pDest)

	// Check it's registered
	if sender.PeerCount() != 1 {
		t.Errorf("PeerCount() = %d after register, want 1", sender.PeerCount())
	}

	dest, ok := sender.GetPeerDestination(nodeID)
	if !ok {
		t.Error("GetPeerDestination() returned false for registered peer")
	}
	if dest != i2pDest {
		t.Errorf("GetPeerDestination() = %q, want %q", dest, i2pDest)
	}

	// Unregister and check
	sender.UnregisterPeer(nodeID)

	if sender.PeerCount() != 0 {
		t.Errorf("PeerCount() = %d after unregister, want 0", sender.PeerCount())
	}

	_, ok = sender.GetPeerDestination(nodeID)
	if ok {
		t.Error("GetPeerDestination() returned true for unregistered peer")
	}
}

func TestSenderSendToUnregisteredPeer(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	err := sender.SendTo("unknown-node", []byte("test"))
	if err == nil {
		t.Error("SendTo() should fail for unregistered peer")
	}
}

func TestSenderBroadcastEmpty(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	// Broadcast with no peers should succeed
	err := sender.Broadcast([]byte("test"))
	if err != nil {
		t.Errorf("Broadcast() with no peers returned error: %v", err)
	}
}

func TestSenderSendToTransportNotOpen(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	// Register a peer
	sender.RegisterPeer("node1", "dest.b32.i2p")

	// SendTo should fail because transport is not open
	err := sender.SendTo("node1", []byte("test"))
	if err == nil {
		t.Error("SendTo() should fail when transport is not open")
	}
}

func TestSenderSendToDestTransportNotOpen(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	// SendToDest should fail because transport is not open
	err := sender.SendToDest("dest.b32.i2p", []byte("test"))
	if err == nil {
		t.Error("SendToDest() should fail when transport is not open")
	}
}

func TestSenderLocalDestinationTransportNil(t *testing.T) {
	// Create sender with nil transport via field manipulation
	sender := &Sender{
		transport:        nil,
		peerDestinations: make(map[string]string),
	}

	_, err := sender.LocalDestination()
	if err == nil {
		t.Error("LocalDestination() should fail when transport is nil")
	}
}

func TestSenderMultiplePeers(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	// Register multiple peers
	peers := map[string]string{
		"node1": "dest1.b32.i2p",
		"node2": "dest2.b32.i2p",
		"node3": "dest3.b32.i2p",
	}

	for nodeID, dest := range peers {
		sender.RegisterPeer(nodeID, dest)
	}

	if sender.PeerCount() != 3 {
		t.Errorf("PeerCount() = %d, want 3", sender.PeerCount())
	}

	// Check each peer
	for nodeID, expectedDest := range peers {
		dest, ok := sender.GetPeerDestination(nodeID)
		if !ok {
			t.Errorf("GetPeerDestination(%q) not found", nodeID)
		}
		if dest != expectedDest {
			t.Errorf("GetPeerDestination(%q) = %q, want %q", nodeID, dest, expectedDest)
		}
	}

	// Unregister one
	sender.UnregisterPeer("node2")

	if sender.PeerCount() != 2 {
		t.Errorf("PeerCount() = %d after unregister, want 2", sender.PeerCount())
	}

	_, ok := sender.GetPeerDestination("node2")
	if ok {
		t.Error("GetPeerDestination(node2) found after unregister")
	}
}

func TestSenderRegisterOverwrite(t *testing.T) {
	trans := NewTransport("test", "127.0.0.1:7656", nil)
	sender := NewSender(trans)

	// Register peer with one destination
	sender.RegisterPeer("node1", "old-dest.b32.i2p")

	// Overwrite with new destination
	sender.RegisterPeer("node1", "new-dest.b32.i2p")

	// Should still have 1 peer
	if sender.PeerCount() != 1 {
		t.Errorf("PeerCount() = %d, want 1", sender.PeerCount())
	}

	// Should have new destination
	dest, _ := sender.GetPeerDestination("node1")
	if dest != "new-dest.b32.i2p" {
		t.Errorf("GetPeerDestination() = %q, want new-dest.b32.i2p", dest)
	}
}
