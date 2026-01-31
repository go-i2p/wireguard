package mesh

import (
	"context"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestNewPeerManager(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "test-node",
		I2PDest:     "test.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
		MaxPeers:    10,
	})

	if pm == nil {
		t.Fatal("NewPeerManager() returned nil")
	}
	if pm.PeerCount() != 0 {
		t.Errorf("PeerCount() = %d, want 0", pm.PeerCount())
	}
}

func TestPeerManager_Handshake(t *testing.T) {
	// Create two peer managers (simulating two nodes)
	key1, _ := wgtypes.GeneratePrivateKey()
	ip1 := AllocateTunnelIP(key1.PublicKey())
	pm1 := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key1.PublicKey(),
		TunnelIP:    ip1,
		NetworkID:   "test-network",
	})

	key2, _ := wgtypes.GeneratePrivateKey()
	ip2 := AllocateTunnelIP(key2.PublicKey())
	pm2 := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-2",
		I2PDest:     "node2.b32.i2p",
		WGPublicKey: key2.PublicKey(),
		TunnelIP:    ip2,
		NetworkID:   "test-network",
	})

	// Add valid token to pm1 (the responder)
	token := []byte("valid-token")
	pm1.AddValidToken(token)

	// Node 2 initiates handshake to Node 1
	init := pm2.CreateHandshakeInit(token)
	if init.NodeID != "node-2" {
		t.Errorf("init.NodeID = %v, want node-2", init.NodeID)
	}

	// Node 1 receives and responds
	resp, err := pm1.HandleHandshakeInit(init)
	if err != nil {
		t.Fatalf("HandleHandshakeInit() error = %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("HandleHandshakeInit() rejected: %s", resp.RejectReason)
	}

	// Node 2 processes response
	err = pm2.HandleHandshakeResponse(resp)
	if err != nil {
		t.Fatalf("HandleHandshakeResponse() error = %v", err)
	}

	// Node 2 sends completion
	complete := pm2.CreateHandshakeComplete(true)
	err = pm1.HandleHandshakeComplete(complete)
	if err != nil {
		t.Fatalf("HandleHandshakeComplete() error = %v", err)
	}

	// Both managers should now know about each other
	if pm1.PeerCount() != 1 {
		t.Errorf("pm1.PeerCount() = %d, want 1", pm1.PeerCount())
	}
	if pm2.PeerCount() != 1 {
		t.Errorf("pm2.PeerCount() = %d, want 1", pm2.PeerCount())
	}

	// Check peer state in pm1
	peer, ok := pm1.GetPeer("node-2")
	if !ok {
		t.Fatal("pm1.GetPeer(node-2) not found")
	}
	if peer.State != PeerStateConnected {
		t.Errorf("peer.State = %v, want Connected", peer.State)
	}
}

func TestPeerManager_InvalidToken(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("correct-token"))

	init := &HandshakeInit{
		I2PDest:     "attacker.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    ip.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("wrong-token"),
		NodeID:      "attacker",
	}

	resp, err := pm.HandleHandshakeInit(init)
	if err != nil {
		t.Fatalf("HandleHandshakeInit() error = %v", err)
	}
	if resp.Accepted {
		t.Error("HandleHandshakeInit() should reject invalid token")
	}
	if resp.RejectReason != "invalid auth token" {
		t.Errorf("RejectReason = %q, want 'invalid auth token'", resp.RejectReason)
	}
}

func TestPeerManager_NetworkMismatch(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "network-a",
	})
	pm.AddValidToken([]byte("token"))

	init := &HandshakeInit{
		I2PDest:     "other.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    ip.String(),
		NetworkID:   "network-b",
		AuthToken:   []byte("token"),
		NodeID:      "other",
	}

	resp, err := pm.HandleHandshakeInit(init)
	if err != nil {
		t.Fatalf("HandleHandshakeInit() error = %v", err)
	}
	if resp.Accepted {
		t.Error("HandleHandshakeInit() should reject network mismatch")
	}
}

func TestPeerManager_ListPeers(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test",
	})
	pm.AddValidToken([]byte("token"))

	// Add a few peers via handshake
	for i := 0; i < 3; i++ {
		peerKey, _ := wgtypes.GeneratePrivateKey()
		peerIP := AllocateTunnelIP(peerKey.PublicKey())
		init := &HandshakeInit{
			I2PDest:     "peer.b32.i2p",
			WGPublicKey: peerKey.PublicKey().String(),
			TunnelIP:    peerIP.String(),
			NetworkID:   "test",
			AuthToken:   []byte("token"),
			NodeID:      string(rune('a' + i)),
		}
		pm.HandleHandshakeInit(init)
	}

	peers := pm.ListPeers()
	if len(peers) != 3 {
		t.Errorf("ListPeers() returned %d peers, want 3", len(peers))
	}
}

func TestPeerManager_GetPeerByIP(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test",
	})
	pm.AddValidToken([]byte("token"))

	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	init := &HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test",
		AuthToken:   []byte("token"),
		NodeID:      "peer-1",
	}
	pm.HandleHandshakeInit(init)

	// Find by IP
	peer, ok := pm.GetPeerByIP(peerIP)
	if !ok {
		t.Fatal("GetPeerByIP() did not find peer")
	}
	if peer.NodeID != "peer-1" {
		t.Errorf("peer.NodeID = %v, want peer-1", peer.NodeID)
	}
}

func TestPeerManager_PruneStale(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test",
	})
	pm.AddValidToken([]byte("token"))

	// Add a peer
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	init := &HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test",
		AuthToken:   []byte("token"),
		NodeID:      "peer-1",
	}
	pm.HandleHandshakeInit(init)

	if pm.PeerCount() != 1 {
		t.Fatalf("PeerCount() = %d, want 1", pm.PeerCount())
	}

	// Prune with very short max age
	pruned := pm.PruneStale(1 * time.Nanosecond)
	if pruned != 1 {
		t.Errorf("PruneStale() = %d, want 1", pruned)
	}
	if pm.PeerCount() != 0 {
		t.Errorf("PeerCount() after prune = %d, want 0", pm.PeerCount())
	}
}

func TestPeerManager_Callbacks(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test",
	})
	pm.AddValidToken([]byte("token"))

	connected := make(chan string, 1)
	disconnected := make(chan string, 1)

	pm.SetCallbacks(
		func(p *Peer) { connected <- p.NodeID },
		func(p *Peer) { disconnected <- p.NodeID },
	)

	// Add and connect peer
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	init := &HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test",
		AuthToken:   []byte("token"),
		NodeID:      "peer-1",
	}
	pm.HandleHandshakeInit(init)
	pm.HandleHandshakeComplete(&HandshakeComplete{NodeID: "peer-1", Success: true})

	select {
	case id := <-connected:
		if id != "peer-1" {
			t.Errorf("connected callback got %v, want peer-1", id)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("connected callback not called")
	}

	// Remove peer
	pm.RemovePeer("peer-1")

	select {
	case id := <-disconnected:
		if id != "peer-1" {
			t.Errorf("disconnected callback got %v, want peer-1", id)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("disconnected callback not called")
	}
}

func TestPeerState_String(t *testing.T) {
	tests := []struct {
		state PeerState
		want  string
	}{
		{PeerStateUnknown, "Unknown"},
		{PeerStatePending, "Pending"},
		{PeerStateConnected, "Connected"},
		{PeerStateDisconnected, "Disconnected"},
		{PeerStateRejected, "Rejected"},
		{PeerState(99), "PeerState(99)"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("PeerState.String() = %v, want %v", got, tt.want)
		}
	}
}

func TestPeerManager_RunHeartbeat(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test",
	})
	pm.AddValidToken([]byte("token"))

	// Add a peer
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	init := &HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test",
		AuthToken:   []byte("token"),
		NodeID:      "peer-1",
	}
	pm.HandleHandshakeInit(init)

	ctx, cancel := context.WithCancel(context.Background())

	// Run heartbeat in background
	go pm.RunHeartbeat(ctx, 10*time.Millisecond, 1*time.Nanosecond)

	// Wait for prune
	time.Sleep(50 * time.Millisecond)
	cancel()

	if pm.PeerCount() != 0 {
		t.Errorf("PeerCount() after heartbeat = %d, want 0", pm.PeerCount())
	}
}

func TestPeerManager_AddValidToken_RejectsEmpty(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "test-node",
		I2PDest:     "test.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})

	// Add empty token - should be silently ignored
	pm.AddValidToken([]byte{})

	// Add nil token - should also be ignored
	pm.AddValidToken(nil)

	// Add a valid token
	pm.AddValidToken([]byte("valid-token"))

	// Create a peer that tries to connect with empty auth token
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())

	init := &HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte{}, // Empty token
		NodeID:      "peer-node",
	}

	resp, err := pm.HandleHandshakeInit(init)
	if err != nil {
		t.Fatalf("HandleHandshakeInit() error = %v", err)
	}

	// Should be rejected because empty token doesn't match any valid token
	if resp.Accepted {
		t.Error("Handshake with empty token should be rejected")
	}
	if resp.RejectReason != "invalid auth token" {
		t.Errorf("RejectReason = %q, want %q", resp.RejectReason, "invalid auth token")
	}
}

func TestPeerManager_RemoveToken(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "test-node",
		I2PDest:     "test.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})

	// Add tokens
	token1 := []byte("token-1")
	token2 := []byte("token-2")
	pm.AddValidToken(token1)
	pm.AddValidToken(token2)

	// Verify token1 works
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	init := &HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test-network",
		AuthToken:   token1,
		NodeID:      "peer-1",
	}
	resp, _ := pm.HandleHandshakeInit(init)
	if !resp.Accepted {
		t.Fatal("token1 should be accepted initially")
	}

	// Remove token1
	pm.RemoveToken(token1)

	// Try to use token1 again with a new peer
	peerKey2, _ := wgtypes.GeneratePrivateKey()
	peerIP2 := AllocateTunnelIP(peerKey2.PublicKey())
	init2 := &HandshakeInit{
		I2PDest:     "peer2.b32.i2p",
		WGPublicKey: peerKey2.PublicKey().String(),
		TunnelIP:    peerIP2.String(),
		NetworkID:   "test-network",
		AuthToken:   token1, // Removed token
		NodeID:      "peer-2",
	}
	resp2, _ := pm.HandleHandshakeInit(init2)
	if resp2.Accepted {
		t.Error("token1 should be rejected after removal")
	}

	// token2 should still work
	peerKey3, _ := wgtypes.GeneratePrivateKey()
	peerIP3 := AllocateTunnelIP(peerKey3.PublicKey())
	init3 := &HandshakeInit{
		I2PDest:     "peer3.b32.i2p",
		WGPublicKey: peerKey3.PublicKey().String(),
		TunnelIP:    peerIP3.String(),
		NetworkID:   "test-network",
		AuthToken:   token2,
		NodeID:      "peer-3",
	}
	resp3, _ := pm.HandleHandshakeInit(init3)
	if !resp3.Accepted {
		t.Error("token2 should still be accepted")
	}

	// Removing empty/nil token should be no-op
	pm.RemoveToken(nil)
	pm.RemoveToken([]byte{})

	// Removing non-existent token should be no-op
	pm.RemoveToken([]byte("nonexistent"))
}

func TestPeerManager_TokenUsedCallback(t *testing.T) {
	// Test that the token used callback is fired when a peer connects using a token
	key1, _ := wgtypes.GeneratePrivateKey()
	ip1 := AllocateTunnelIP(key1.PublicKey())
	pm1 := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key1.PublicKey(),
		TunnelIP:    ip1,
		NetworkID:   "test-network",
	})

	// Track token usage via callback
	var usedToken []byte
	callbackCalled := make(chan struct{}, 1)
	pm1.SetTokenUsedCallback(func(token []byte) {
		usedToken = token
		callbackCalled <- struct{}{}
	})

	// Add a valid token
	token := []byte("invite-token-12345")
	pm1.AddValidToken(token)

	// Create initiator
	key2, _ := wgtypes.GeneratePrivateKey()
	ip2 := AllocateTunnelIP(key2.PublicKey())
	pm2 := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-2",
		I2PDest:     "node2.b32.i2p",
		WGPublicKey: key2.PublicKey(),
		TunnelIP:    ip2,
		NetworkID:   "test-network",
	})

	// Perform handshake
	init := pm2.CreateHandshakeInit(token)
	resp, err := pm1.HandleHandshakeInit(init)
	if err != nil || !resp.Accepted {
		t.Fatalf("HandleHandshakeInit() failed: %v, accepted=%v", err, resp.Accepted)
	}

	err = pm2.HandleHandshakeResponse(resp)
	if err != nil {
		t.Fatalf("HandleHandshakeResponse() error = %v", err)
	}

	complete := pm2.CreateHandshakeComplete(true)
	err = pm1.HandleHandshakeComplete(complete)
	if err != nil {
		t.Fatalf("HandleHandshakeComplete() error = %v", err)
	}

	// Wait for callback (with timeout)
	select {
	case <-callbackCalled:
		// Success - callback was called
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Token used callback was not called")
	}

	// Verify the correct token was reported
	if string(usedToken) != string(token) {
		t.Errorf("Token used callback received wrong token: got %q, want %q", usedToken, token)
	}

	// Verify peer has the token stored
	peer, ok := pm1.GetPeer("node-2")
	if !ok {
		t.Fatal("Peer not found after handshake")
	}
	if string(peer.AuthToken) != string(token) {
		t.Errorf("Peer.AuthToken = %q, want %q", peer.AuthToken, token)
	}
}

func TestPeerManager_TokenUsedCallback_NotCalledWithoutToken(t *testing.T) {
	// Test that callback is NOT called when peer connects without a tracked token
	// (This could happen with discovery tokens or existing peers)
	key1, _ := wgtypes.GeneratePrivateKey()
	ip1 := AllocateTunnelIP(key1.PublicKey())
	pm1 := NewPeerManager(PeerManagerConfig{
		NodeID:      "node-1",
		I2PDest:     "node1.b32.i2p",
		WGPublicKey: key1.PublicKey(),
		TunnelIP:    ip1,
		NetworkID:   "test-network",
	})

	callbackCalled := false
	pm1.SetTokenUsedCallback(func(token []byte) {
		callbackCalled = true
	})

	// Manually add a peer without going through handshake
	// (simulating an existing/reconnecting peer)
	key2, _ := wgtypes.GeneratePrivateKey()
	ip2 := AllocateTunnelIP(key2.PublicKey())

	// Complete handshake for a peer with no stored token
	pm1.HandleHandshakeComplete(&HandshakeComplete{
		NodeID:  "node-2",
		Success: true,
	})

	// Give callback time to potentially fire
	time.Sleep(20 * time.Millisecond)

	// Callback should not have been called (unknown peer, no token)
	// Note: This returns an error because peer is unknown, which is expected
	if callbackCalled {
		t.Error("Token used callback should not be called for unknown peer")
	}

	// Now test with a peer that exists but has no AuthToken
	pm1.AddValidToken([]byte("token"))

	// Add peer directly to simulate reconnection scenario
	init := &HandshakeInit{
		I2PDest:     "node2.b32.i2p",
		WGPublicKey: key2.PublicKey().String(),
		TunnelIP:    ip2.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("token"),
		NodeID:      "node-2",
	}
	resp, _ := pm1.HandleHandshakeInit(init)
	if resp.Accepted {
		// Clear the auth token to simulate a reconnecting peer
		pm1.mu.Lock()
		if p, ok := pm1.peers["node-2"]; ok {
			p.AuthToken = nil // Clear token
		}
		pm1.mu.Unlock()

		callbackCalled = false
		pm1.HandleHandshakeComplete(&HandshakeComplete{
			NodeID:  "node-2",
			Success: true,
		})

		time.Sleep(20 * time.Millisecond)

		if callbackCalled {
			t.Error("Token used callback should not be called for peer with nil AuthToken")
		}
	}
}

// TestHandshakeInit_EmptyAuthToken tests that handshakes with empty auth tokens are rejected.
// This test addresses the AUDIT.md finding: "Add Empty Token Validation"
func TestHandshakeInit_EmptyAuthToken(t *testing.T) {
	// Create peer manager (responder)
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "responder-node",
		I2PDest:     "responder.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})

	// Add a valid token (but we'll send empty token in handshake)
	validToken := []byte("valid-token-12345")
	pm.AddValidToken(validToken)

	// Create initiator identity
	initKey, _ := wgtypes.GeneratePrivateKey()
	initIP := AllocateTunnelIP(initKey.PublicKey())
	initNodeID := "initiator-node"

	tests := []struct {
		name      string
		authToken []byte
		wantError bool
	}{
		{
			name:      "empty token (nil)",
			authToken: nil,
			wantError: true,
		},
		{
			name:      "empty token (zero-length slice)",
			authToken: []byte{},
			wantError: true,
		},
		{
			name:      "valid non-empty token",
			authToken: validToken,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create handshake init with specified auth token
			init := &HandshakeInit{
				I2PDest:     "initiator.b32.i2p",
				WGPublicKey: initKey.PublicKey().String(),
				TunnelIP:    initIP.String(),
				NetworkID:   "test-network",
				AuthToken:   tt.authToken,
				NodeID:      initNodeID,
			}

			// Handle the handshake
			resp, err := pm.HandleHandshakeInit(init)
			if err != nil {
				t.Fatalf("HandleHandshakeInit() unexpected error: %v", err)
			}

			if tt.wantError {
				// Should be rejected
				if resp.Accepted {
					t.Error("HandleHandshakeInit() accepted empty auth token, expected rejection")
				}
				if resp.RejectReason == "" {
					t.Error("HandleHandshakeInit() rejected but no reason provided")
				}
				// Verify the rejection reason mentions empty token
				if resp.RejectReason != "empty auth token" && resp.RejectReason != "invalid auth token" {
					t.Errorf("HandleHandshakeInit() reject reason = %q, expected 'empty auth token' or 'invalid auth token'", resp.RejectReason)
				}
			} else {
				// Should be accepted
				if !resp.Accepted {
					t.Errorf("HandleHandshakeInit() rejected valid token: %s", resp.RejectReason)
				}
			}
		})
	}
}

// TestAddValidToken_EmptyToken tests that empty tokens are silently ignored.
func TestAddValidToken_EmptyToken(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "test-node",
		I2PDest:     "test.b32.i2p",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})

	// Add empty tokens (should be silently ignored)
	pm.AddValidToken(nil)
	pm.AddValidToken([]byte{})

	// Add a valid token
	validToken := []byte("valid-token")
	pm.AddValidToken(validToken)

	// Check that only the valid token is in the list
	pm.mu.RLock()
	tokenCount := len(pm.validTokens)
	pm.mu.RUnlock()

	if tokenCount != 1 {
		t.Errorf("validTokens count = %d, want 1 (empty tokens should be ignored)", tokenCount)
	}

	// Verify the valid token works
	_, valid := pm.validateToken(validToken)
	if !valid {
		t.Error("validateToken() failed for valid token")
	}

	// Verify empty tokens don't validate (even though they weren't added)
	_, valid = pm.validateToken(nil)
	if valid {
		t.Error("validateToken() unexpectedly accepted nil token")
	}

	_, valid = pm.validateToken([]byte{})
	if valid {
		t.Error("validateToken() unexpectedly accepted empty token")
	}
}
