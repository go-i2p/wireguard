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
