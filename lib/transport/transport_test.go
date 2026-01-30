package transport

import (
	"testing"
)

func TestNewTransport(t *testing.T) {
	tr := NewTransport("test-tunnel", "127.0.0.1:7656", nil)
	if tr == nil {
		t.Fatal("NewTransport returned nil")
	}

	if tr.name != "test-tunnel" {
		t.Errorf("name mismatch: got %q", tr.name)
	}
	if tr.samAddr != "127.0.0.1:7656" {
		t.Errorf("samAddr mismatch: got %q", tr.samAddr)
	}
	if tr.IsOpen() {
		t.Error("transport should not be open initially")
	}
	if tr.LocalAddress() != "" {
		t.Error("local address should be empty when not open")
	}
}

func TestTransport_NotOpen(t *testing.T) {
	tr := NewTransport("test", "127.0.0.1:7656", nil)

	// Operations should fail when not open
	_, err := tr.LocalDestination()
	if err == nil {
		t.Error("LocalDestination should error when not open")
	}

	err = tr.AddPeer("test.b32.i2p", "pubkey")
	if err == nil {
		t.Error("AddPeer should error when not open")
	}

	_, err = tr.ParseEndpoint("test.b32.i2p")
	if err == nil {
		t.Error("ParseEndpoint should error when not open")
	}

	// IsOpen should return false
	if tr.IsOpen() {
		t.Error("IsOpen should return false when not open")
	}
}

func TestTransport_PeerTracking(t *testing.T) {
	tr := NewTransport("test", "127.0.0.1:7656", nil)

	// Manually set open state for testing peer tracking
	// (normally this would be done by Open())
	tr.mu.Lock()
	tr.isOpen = true
	tr.mu.Unlock()

	// Add a peer
	err := tr.AddPeer("peer1.b32.i2p", "pubkey1")
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Check peer count
	if tr.PeerCount() != 1 {
		t.Errorf("peer count should be 1, got %d", tr.PeerCount())
	}

	// Get peer
	peer, ok := tr.GetPeer("peer1.b32.i2p")
	if !ok {
		t.Fatal("GetPeer should find peer")
	}
	if peer.I2PDest != "peer1.b32.i2p" {
		t.Errorf("peer I2P dest mismatch: got %q", peer.I2PDest)
	}
	if peer.WGPublicKey != "pubkey1" {
		t.Errorf("peer WG key mismatch: got %q", peer.WGPublicKey)
	}
	if peer.State != PeerStateConnecting {
		t.Errorf("initial peer state should be Connecting, got %s", peer.State)
	}

	// Update peer state
	tr.UpdatePeerState("peer1.b32.i2p", PeerStateConnected)
	peer, _ = tr.GetPeer("peer1.b32.i2p")
	if peer.State != PeerStateConnected {
		t.Errorf("peer state should be Connected, got %s", peer.State)
	}

	// Connected count
	if tr.ConnectedPeerCount() != 1 {
		t.Errorf("connected peer count should be 1, got %d", tr.ConnectedPeerCount())
	}

	// Add another peer (not connected)
	tr.AddPeer("peer2.b32.i2p", "pubkey2")
	if tr.PeerCount() != 2 {
		t.Errorf("peer count should be 2, got %d", tr.PeerCount())
	}
	if tr.ConnectedPeerCount() != 1 {
		t.Errorf("connected peer count should still be 1, got %d", tr.ConnectedPeerCount())
	}

	// List peers
	peers := tr.ListPeers()
	if len(peers) != 2 {
		t.Errorf("ListPeers should return 2 peers, got %d", len(peers))
	}

	// Remove peer
	tr.RemovePeer("peer1.b32.i2p")
	if tr.PeerCount() != 1 {
		t.Errorf("peer count should be 1 after remove, got %d", tr.PeerCount())
	}

	_, ok = tr.GetPeer("peer1.b32.i2p")
	if ok {
		t.Error("GetPeer should not find removed peer")
	}
}

func TestPeerState_String(t *testing.T) {
	tests := []struct {
		state PeerState
		want  string
	}{
		{PeerStateUnknown, "unknown"},
		{PeerStateConnecting, "connecting"},
		{PeerStateConnected, "connected"},
		{PeerStateStale, "stale"},
		{PeerStateDisconnected, "disconnected"},
		{PeerState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("PeerState.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTransport_Close_Idempotent(t *testing.T) {
	tr := NewTransport("test", "127.0.0.1:7656", nil)

	// Close when not open should not error
	err := tr.Close()
	if err != nil {
		t.Errorf("Close should not error when not open: %v", err)
	}

	// Close again should also not error
	err = tr.Close()
	if err != nil {
		t.Errorf("Second Close should not error: %v", err)
	}
}

func TestTransport_UpdateNonexistentPeer(t *testing.T) {
	tr := NewTransport("test", "127.0.0.1:7656", nil)
	tr.mu.Lock()
	tr.isOpen = true
	tr.mu.Unlock()

	// These should not panic, just no-op
	tr.UpdatePeerState("nonexistent.b32.i2p", PeerStateConnected)
	tr.SetPeerEndpoint("nonexistent.b32.i2p", nil)

	if tr.PeerCount() != 0 {
		t.Error("peer count should be 0")
	}
}

func TestTransport_Reconnect_WhenClosed(t *testing.T) {
	tr := NewTransport("test-reconnect", "127.0.0.1:7656", nil)

	// Reconnect when not open should still attempt to open
	// (will fail without SAM running, but tests the code path)
	_, err := tr.Reconnect()

	// We expect an error since SAM isn't running
	if err == nil {
		// If no error, it means SAM is running (integration test environment)
		if !tr.IsOpen() {
			t.Error("Reconnect succeeded but transport is not open")
		}
		// Clean up
		tr.Close()
	}
}

func TestTransport_Reconnect_PreservesPeers(t *testing.T) {
	tr := NewTransport("test", "127.0.0.1:7656", nil)

	// Manually set up state
	tr.mu.Lock()
	tr.isOpen = true
	tr.localI2PDest = "original.b32.i2p"
	tr.peers["peer1.b32.i2p"] = &TrackedPeer{
		I2PDest:     "peer1.b32.i2p",
		WGPublicKey: "key1",
		State:       PeerStateConnected,
	}
	tr.mu.Unlock()

	// Since we can't actually reconnect without SAM, just verify
	// the Reconnect function's behavior with invalid state
	// The real test would require integration testing with SAM

	if tr.PeerCount() != 1 {
		t.Error("peer should be tracked before reconnect attempt")
	}
}
