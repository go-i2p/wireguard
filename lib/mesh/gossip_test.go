package mesh

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// mockSender implements MessageSender for testing.
type mockSender struct {
	mu         sync.Mutex
	broadcasts [][]byte
	sends      map[string][][]byte
	destSends  map[string][][]byte
}

func newMockSender() *mockSender {
	return &mockSender{
		sends:     make(map[string][][]byte),
		destSends: make(map[string][][]byte),
	}
}

func (m *mockSender) SendTo(nodeID string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sends[nodeID] = append(m.sends[nodeID], data)
	return nil
}

func (m *mockSender) SendToDest(i2pDest string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destSends[i2pDest] = append(m.destSends[i2pDest], data)
	return nil
}

func (m *mockSender) Broadcast(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcasts = append(m.broadcasts, data)
	return nil
}

func (m *mockSender) getBroadcastCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.broadcasts)
}

func (m *mockSender) getSendCount(nodeID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sends[nodeID])
}

func (m *mockSender) getDestSendCount(i2pDest string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.destSends[i2pDest])
}

// mockFailingSender is a sender that fails SendTo but succeeds on SendToDest.
// This is used to test the fallback behavior in handshake init handling.
type mockFailingSender struct {
	mu         sync.Mutex
	destSends  map[string][][]byte
	broadcasts [][]byte
}

func newMockFailingSender() *mockFailingSender {
	return &mockFailingSender{
		destSends: make(map[string][][]byte),
	}
}

func (m *mockFailingSender) SendTo(nodeID string, data []byte) error {
	// Always fail to simulate unregistered peer
	return errors.New("peer not registered for sending")
}

func (m *mockFailingSender) SendToDest(i2pDest string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destSends[i2pDest] = append(m.destSends[i2pDest], data)
	return nil
}

func (m *mockFailingSender) Broadcast(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcasts = append(m.broadcasts, data)
	return nil
}

func (m *mockFailingSender) getDestSendCount(i2pDest string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.destSends[i2pDest])
}

func TestDefaultGossipConfig(t *testing.T) {
	cfg := DefaultGossipConfig()

	if cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("HeartbeatInterval = %v, want 30s", cfg.HeartbeatInterval)
	}
	if cfg.GossipInterval != 60*time.Second {
		t.Errorf("GossipInterval = %v, want 60s", cfg.GossipInterval)
	}
	if cfg.FanOut != 3 {
		t.Errorf("FanOut = %d, want 3", cfg.FanOut)
	}
}

func TestNewGossipEngine(t *testing.T) {
	sender := newMockSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "test-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})

	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	ge := NewGossipEngine(GossipEngineConfig{
		Config:       DefaultGossipConfig(),
		PeerManager:  pm,
		RoutingTable: rt,
		Sender:       sender,
		NodeID:       "test-node",
		I2PDest:      "test.b32.i2p",
		WGPublicKey:  key.PublicKey().String(),
		TunnelIP:     ip.String(),
		NetworkID:    "test-network",
	})

	if ge == nil {
		t.Fatal("NewGossipEngine() returned nil")
	}
	if ge.IsRunning() {
		t.Error("IsRunning() = true before Start()")
	}
}

func TestGossipEngine_StartStop(t *testing.T) {
	sender := newMockSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	ge := NewGossipEngine(GossipEngineConfig{
		Config: GossipConfig{
			HeartbeatInterval:   10 * time.Millisecond,
			GossipInterval:      20 * time.Millisecond,
			AntiEntropyInterval: 50 * time.Millisecond,
			PeerTimeout:         100 * time.Millisecond,
			FanOut:              2,
		},
		Sender:      sender,
		NodeID:      "test-node",
		I2PDest:     "test.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    ip.String(),
		NetworkID:   "test-network",
	})

	ctx := context.Background()
	err := ge.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !ge.IsRunning() {
		t.Error("IsRunning() = false after Start()")
	}

	// Wait for at least one heartbeat
	time.Sleep(50 * time.Millisecond)

	ge.Stop()

	if ge.IsRunning() {
		t.Error("IsRunning() = true after Stop()")
	}

	// Should have sent at least one broadcast (initial announcement)
	if sender.getBroadcastCount() == 0 {
		t.Error("No broadcasts sent during gossip")
	}
}

func TestGossipEngine_DoubleStart(t *testing.T) {
	sender := newMockSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	ge := NewGossipEngine(GossipEngineConfig{
		Config:      DefaultGossipConfig(),
		Sender:      sender,
		NodeID:      "test-node",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    ip.String(),
		NetworkID:   "test-network",
	})

	ctx := context.Background()
	ge.Start(ctx)
	defer ge.Stop()

	// Second start should be no-op
	err := ge.Start(ctx)
	if err != nil {
		t.Errorf("Second Start() error = %v", err)
	}
}

func TestGossipEngine_HandlePeerAnnounce(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("token"))

	// Add a peer first
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	pm.HandleHandshakeInit(&HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("token"),
		NodeID:      "remote-node",
	})
	pm.HandleHandshakeComplete(&HandshakeComplete{NodeID: "remote-node", Success: true})

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager: pm,
		NodeID:      "local-node",
		NetworkID:   "test-network",
	})

	// Create announcement message
	announce := &PeerAnnounce{
		NodeID:    "remote-node",
		NetworkID: "test-network",
		PeerCount: 5,
	}
	data, _ := EncodeMessage(MsgPeerAnnounce, announce)
	msg, _ := DecodeMessage(data)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
	}
}

func TestGossipEngine_HandlePeerAnnounce_IgnoreOwn(t *testing.T) {
	ge := NewGossipEngine(GossipEngineConfig{
		NodeID:    "my-node",
		NetworkID: "test-network",
	})

	// Our own announcement should be ignored
	announce := &PeerAnnounce{
		NodeID:    "my-node",
		NetworkID: "test-network",
	}
	data, _ := EncodeMessage(MsgPeerAnnounce, announce)
	msg, _ := DecodeMessage(data)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
	}
}

func TestGossipEngine_HandlePeerAnnounce_IgnoreDifferentNetwork(t *testing.T) {
	ge := NewGossipEngine(GossipEngineConfig{
		NodeID:    "my-node",
		NetworkID: "network-a",
	})

	// Different network should be ignored
	announce := &PeerAnnounce{
		NodeID:    "other-node",
		NetworkID: "network-b",
	}
	data, _ := EncodeMessage(MsgPeerAnnounce, announce)
	msg, _ := DecodeMessage(data)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
	}
}

func TestGossipEngine_HandlePeerList(t *testing.T) {
	ge := NewGossipEngine(GossipEngineConfig{
		NodeID:    "my-node",
		NetworkID: "test-network",
	})

	peerList := &PeerListPayload{
		NetworkID: "test-network",
		NodeID:    "other-node",
		Peers: []PeerInfo{
			{NodeID: "peer-1", TunnelIP: "10.42.1.1"},
			{NodeID: "peer-2", TunnelIP: "10.42.1.2"},
		},
	}
	data, _ := EncodeMessage(MsgPeerList, peerList)
	msg, _ := DecodeMessage(data)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
	}
}

func TestGossipEngine_HandlePeerList_DiscoveryCallback(t *testing.T) {
	// Create peer manager to track known peers
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())
	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "my-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})

	ge := NewGossipEngine(GossipEngineConfig{
		NodeID:      "my-node",
		NetworkID:   "test-network",
		PeerManager: pm,
	})

	// Track discovered peers
	var discoveredPeers []PeerInfo
	var mu sync.Mutex
	ge.SetDiscoveryCallback(func(info PeerInfo) {
		mu.Lock()
		discoveredPeers = append(discoveredPeers, info)
		mu.Unlock()
	})

	// Send a peer list with unknown peers
	peerList := &PeerListPayload{
		NetworkID: "test-network",
		NodeID:    "other-node",
		Peers: []PeerInfo{
			{NodeID: "new-peer-1", I2PDest: "dest1.b32.i2p", TunnelIP: "10.42.1.1"},
			{NodeID: "new-peer-2", I2PDest: "dest2.b32.i2p", TunnelIP: "10.42.1.2"},
			{NodeID: "my-node", TunnelIP: "10.42.0.1"}, // Should be skipped (ourselves)
		},
	}
	data, _ := EncodeMessage(MsgPeerList, peerList)
	msg, _ := DecodeMessage(data)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
	}

	// Check that callback was invoked for the two new peers (not ourselves)
	mu.Lock()
	defer mu.Unlock()
	if len(discoveredPeers) != 2 {
		t.Errorf("Expected 2 discovered peers, got %d", len(discoveredPeers))
	}

	// Verify the discovered peers
	foundPeer1 := false
	foundPeer2 := false
	for _, p := range discoveredPeers {
		if p.NodeID == "new-peer-1" {
			foundPeer1 = true
		}
		if p.NodeID == "new-peer-2" {
			foundPeer2 = true
		}
	}
	if !foundPeer1 || !foundPeer2 {
		t.Errorf("Expected to find new-peer-1 and new-peer-2, got %+v", discoveredPeers)
	}
}

func TestGossipEngine_HandlePeerLeave(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("token"))

	// Add a peer
	peerKey, _ := wgtypes.GeneratePrivateKey()
	peerIP := AllocateTunnelIP(peerKey.PublicKey())
	pm.HandleHandshakeInit(&HandshakeInit{
		I2PDest:     "peer.b32.i2p",
		WGPublicKey: peerKey.PublicKey().String(),
		TunnelIP:    peerIP.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("token"),
		NodeID:      "leaving-node",
	})

	rt := NewRoutingTable(RoutingTableConfig{Subnet: TunnelSubnet})
	rt.AddRoute(&RouteEntry{
		TunnelIP: peerIP,
		NodeID:   "leaving-node",
	})

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager:  pm,
		RoutingTable: rt,
		NodeID:       "local-node",
		NetworkID:    "test-network",
	})

	// Peer leaving
	leave := &PeerLeavePayload{
		NodeID:    "leaving-node",
		NetworkID: "test-network",
		Reason:    "shutdown",
	}
	data, _ := EncodeMessage(MsgPeerLeave, leave)
	msg, _ := DecodeMessage(data)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
	}

	// Peer should be removed
	if pm.PeerCount() != 0 {
		t.Errorf("Peer was not removed, count = %d", pm.PeerCount())
	}

	if rt.RouteCount() != 0 {
		t.Errorf("Route was not removed, count = %d", rt.RouteCount())
	}
}

func TestGossipEngine_AnnounceLeave(t *testing.T) {
	sender := newMockSender()

	ge := NewGossipEngine(GossipEngineConfig{
		Sender:    sender,
		NodeID:    "my-node",
		NetworkID: "test-network",
	})

	ge.AnnounceLeave("shutting down")

	if sender.getBroadcastCount() != 1 {
		t.Errorf("AnnounceLeave() broadcast count = %d, want 1", sender.getBroadcastCount())
	}

	// Verify the message content
	data := sender.broadcasts[0]
	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if msg.Type != MsgPeerLeave {
		t.Errorf("Message type = %v, want MsgPeerLeave", msg.Type)
	}
}

func TestGossipEngine_AnnouncePresence(t *testing.T) {
	sender := newMockSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	ge := NewGossipEngine(GossipEngineConfig{
		Sender:      sender,
		NodeID:      "my-node",
		NetworkID:   "test-network",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    ip.String(),
		I2PDest:     "test.b32.i2p",
	})

	// AnnouncePresence should do nothing when not running
	ge.AnnouncePresence()
	if sender.getBroadcastCount() != 0 {
		t.Errorf("AnnouncePresence() when not running should not broadcast, got %d", sender.getBroadcastCount())
	}

	// Start the engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ge.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ge.Stop()

	// Clear any broadcasts from Start
	sender.mu.Lock()
	sender.broadcasts = nil
	sender.mu.Unlock()

	// Now AnnouncePresence should work
	ge.AnnouncePresence()
	if sender.getBroadcastCount() != 1 {
		t.Errorf("AnnouncePresence() broadcast count = %d, want 1", sender.getBroadcastCount())
	}

	// Verify the message content
	sender.mu.Lock()
	data := sender.broadcasts[0]
	sender.mu.Unlock()

	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if msg.Type != MsgPeerAnnounce {
		t.Errorf("Message type = %v, want MsgPeerAnnounce", msg.Type)
	}
}

func TestGossipEngine_SelectRandomPeers(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("token"))

	// Add multiple peers
	for i := 0; i < 5; i++ {
		peerKey, _ := wgtypes.GeneratePrivateKey()
		peerIP := AllocateTunnelIP(peerKey.PublicKey())
		pm.HandleHandshakeInit(&HandshakeInit{
			I2PDest:     "peer.b32.i2p",
			WGPublicKey: peerKey.PublicKey().String(),
			TunnelIP:    peerIP.String(),
			NetworkID:   "test-network",
			AuthToken:   []byte("token"),
			NodeID:      string(rune('a' + i)),
		})
		pm.HandleHandshakeComplete(&HandshakeComplete{NodeID: string(rune('a' + i)), Success: true})
	}

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager: pm,
		NodeID:      "local-node",
		NetworkID:   "test-network",
	})

	// Select 3 random peers
	selected := ge.selectRandomPeers(3)
	if len(selected) != 3 {
		t.Errorf("selectRandomPeers(3) returned %d peers, want 3", len(selected))
	}

	// Request more than available
	selected = ge.selectRandomPeers(10)
	if len(selected) != 5 {
		t.Errorf("selectRandomPeers(10) returned %d peers, want 5", len(selected))
	}
}

func TestGossipEngine_NilDependencies(t *testing.T) {
	// Engine should handle nil dependencies gracefully
	ge := NewGossipEngine(GossipEngineConfig{
		NodeID:    "test-node",
		NetworkID: "test-network",
	})

	ctx := context.Background()
	err := ge.Start(ctx)
	if err != nil {
		t.Fatalf("Start() with nil deps error = %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	ge.Stop()
}

func TestGossipEngine_HandleUnknownMessage(t *testing.T) {
	ge := NewGossipEngine(GossipEngineConfig{
		NodeID:    "test-node",
		NetworkID: "test-network",
	})

	msg := &Message{
		Type: MessageType(99), // Unknown type
	}

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage() for unknown type should not error, got %v", err)
	}
}

func TestGossipEngine_HandleHandshakeInit(t *testing.T) {
	sender := newMockSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("valid-token"))

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager: pm,
		Sender:      sender,
		NodeID:      "local-node",
		I2PDest:     "local.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    ip.String(),
		NetworkID:   "test-network",
	})

	// Register the peer so response can be sent
	sender.RegisterPeer("remote-node", "remote.b32.i2p")

	// Create handshake init message from a remote peer
	remoteKey, _ := wgtypes.GeneratePrivateKey()
	remoteIP := AllocateTunnelIP(remoteKey.PublicKey())

	init := &HandshakeInit{
		I2PDest:     "remote.b32.i2p",
		WGPublicKey: remoteKey.PublicKey().String(),
		TunnelIP:    remoteIP.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("valid-token"),
		NodeID:      "remote-node",
	}

	payload, _ := EncodeMessage(MsgHandshakeInit, init)
	msg, _ := DecodeMessage(payload)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage(HandshakeInit) error = %v", err)
	}

	// Check that peer was added to manager
	peer, found := pm.GetPeer("remote-node")
	if !found {
		t.Error("Peer not added to manager after handshake init")
	} else if peer.State != PeerStatePending {
		t.Errorf("Peer state = %v, want Pending", peer.State)
	}
}

func TestGossipEngine_HandleHandshakeInit_InvalidToken(t *testing.T) {
	sender := newMockSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("valid-token"))

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager: pm,
		Sender:      sender,
		NodeID:      "local-node",
		NetworkID:   "test-network",
	})

	// Create handshake init with invalid token
	remoteKey, _ := wgtypes.GeneratePrivateKey()
	remoteIP := AllocateTunnelIP(remoteKey.PublicKey())

	init := &HandshakeInit{
		I2PDest:     "remote.b32.i2p",
		WGPublicKey: remoteKey.PublicKey().String(),
		TunnelIP:    remoteIP.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("invalid-token"),
		NodeID:      "remote-node",
	}

	payload, _ := EncodeMessage(MsgHandshakeInit, init)
	msg, _ := DecodeMessage(payload)

	// Should not error - rejection is handled via response
	err := ge.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage(HandshakeInit) error = %v", err)
	}

	// Peer should not be in connected state
	peer, found := pm.GetPeer("remote-node")
	if found && peer.State == PeerStateConnected {
		t.Error("Peer should not be connected with invalid token")
	}
}

// TestGossipEngine_HandleHandshakeInit_Fallback tests that when SendTo fails
// (because the peer isn't registered yet), the response is sent via SendToDest
// using the I2P destination from the handshake init message.
func TestGossipEngine_HandleHandshakeInit_Fallback(t *testing.T) {
	sender := newMockFailingSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("valid-token"))

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager: pm,
		Sender:      sender,
		NodeID:      "local-node",
		I2PDest:     "local.b32.i2p",
		WGPublicKey: key.PublicKey().String(),
		TunnelIP:    ip.String(),
		NetworkID:   "test-network",
	})

	// Create handshake init message from a remote peer
	remoteKey, _ := wgtypes.GeneratePrivateKey()
	remoteIP := AllocateTunnelIP(remoteKey.PublicKey())
	remoteI2PDest := "remote-peer.b32.i2p"

	init := &HandshakeInit{
		I2PDest:     remoteI2PDest,
		WGPublicKey: remoteKey.PublicKey().String(),
		TunnelIP:    remoteIP.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("valid-token"),
		NodeID:      "remote-node",
	}

	payload, _ := EncodeMessage(MsgHandshakeInit, init)
	msg, _ := DecodeMessage(payload)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage(HandshakeInit) error = %v", err)
	}

	// Check that the fallback SendToDest was called with the remote I2P destination
	destSendCount := sender.getDestSendCount(remoteI2PDest)
	if destSendCount != 1 {
		t.Errorf("SendToDest call count = %d, want 1 (fallback should be used)", destSendCount)
	}

	// Verify peer was still added to manager
	peer, found := pm.GetPeer("remote-node")
	if !found {
		t.Error("Peer not added to manager after handshake init")
	} else if peer.State != PeerStatePending {
		t.Errorf("Peer state = %v, want Pending", peer.State)
	}
}

func TestGossipEngine_HandleHandshakeResponse(t *testing.T) {
	sender := newMockSender()
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager: pm,
		Sender:      sender,
		NodeID:      "local-node",
		NetworkID:   "test-network",
	})

	// Create handshake response from a remote peer
	remoteKey, _ := wgtypes.GeneratePrivateKey()
	remoteIP := AllocateTunnelIP(remoteKey.PublicKey())

	// Register peer for sending completion
	sender.RegisterPeer("remote-node", "remote.b32.i2p")

	response := &HandshakeResponse{
		I2PDest:     "remote.b32.i2p",
		WGPublicKey: remoteKey.PublicKey().String(),
		TunnelIP:    remoteIP.String(),
		NetworkID:   "test-network",
		NodeID:      "remote-node",
		Accepted:    true,
	}

	payload, _ := EncodeMessage(MsgHandshakeResponse, response)
	msg, _ := DecodeMessage(payload)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage(HandshakeResponse) error = %v", err)
	}

	// Check that peer was added to manager
	peer, found := pm.GetPeer("remote-node")
	if !found {
		t.Error("Peer not added to manager after handshake response")
	} else if peer.State != PeerStatePending {
		t.Errorf("Peer state = %v, want Pending", peer.State)
	}
}

func TestGossipEngine_HandleHandshakeComplete(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	pm := NewPeerManager(PeerManagerConfig{
		NodeID:      "local-node",
		WGPublicKey: key.PublicKey(),
		TunnelIP:    ip,
		NetworkID:   "test-network",
	})
	pm.AddValidToken([]byte("valid-token"))

	// First add a pending peer via handshake init
	remoteKey, _ := wgtypes.GeneratePrivateKey()
	remoteIP := AllocateTunnelIP(remoteKey.PublicKey())

	pm.HandleHandshakeInit(&HandshakeInit{
		I2PDest:     "remote.b32.i2p",
		WGPublicKey: remoteKey.PublicKey().String(),
		TunnelIP:    remoteIP.String(),
		NetworkID:   "test-network",
		AuthToken:   []byte("valid-token"),
		NodeID:      "remote-node",
	})

	ge := NewGossipEngine(GossipEngineConfig{
		PeerManager: pm,
		NodeID:      "local-node",
		NetworkID:   "test-network",
	})

	// Create handshake complete
	complete := &HandshakeComplete{
		NodeID:  "remote-node",
		Success: true,
	}

	payload, _ := EncodeMessage(MsgHandshakeComplete, complete)
	msg, _ := DecodeMessage(payload)

	err := ge.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage(HandshakeComplete) error = %v", err)
	}

	// Check that peer is now connected
	peer, found := pm.GetPeer("remote-node")
	if !found {
		t.Fatal("Peer not found after handshake complete")
	}
	if peer.State != PeerStateConnected {
		t.Errorf("Peer state = %v, want Connected", peer.State)
	}
}

func TestGossipEngine_SetHandshakeCallbacks(t *testing.T) {
	ge := NewGossipEngine(GossipEngineConfig{
		NodeID:    "test-node",
		NetworkID: "test-network",
	})

	initCalled := false
	responseCalled := false
	completeCalled := false

	ge.SetHandshakeCallbacks(
		func(init *HandshakeInit) (*HandshakeResponse, error) {
			initCalled = true
			return &HandshakeResponse{Accepted: true}, nil
		},
		func(resp *HandshakeResponse) error {
			responseCalled = true
			return nil
		},
		func(complete *HandshakeComplete) error {
			completeCalled = true
			return nil
		},
	)

	// Test that custom callbacks are used
	remoteKey, _ := wgtypes.GeneratePrivateKey()
	remoteIP := AllocateTunnelIP(remoteKey.PublicKey())

	// Handle init
	initPayload, _ := EncodeMessage(MsgHandshakeInit, &HandshakeInit{
		NodeID:      "remote",
		WGPublicKey: remoteKey.PublicKey().String(),
		TunnelIP:    remoteIP.String(),
		NetworkID:   "test-network",
	})
	initMsg, _ := DecodeMessage(initPayload)
	ge.HandleMessage(initMsg)

	if !initCalled {
		t.Error("Custom init callback not called")
	}

	// Handle response
	respPayload, _ := EncodeMessage(MsgHandshakeResponse, &HandshakeResponse{
		NodeID:      "remote",
		WGPublicKey: remoteKey.PublicKey().String(),
		TunnelIP:    remoteIP.String(),
		NetworkID:   "test-network",
		Accepted:    true,
	})
	respMsg, _ := DecodeMessage(respPayload)
	ge.HandleMessage(respMsg)

	if !responseCalled {
		t.Error("Custom response callback not called")
	}

	// Handle complete
	completePayload, _ := EncodeMessage(MsgHandshakeComplete, &HandshakeComplete{
		NodeID:  "remote",
		Success: true,
	})
	completeMsg, _ := DecodeMessage(completePayload)
	ge.HandleMessage(completeMsg)

	if !completeCalled {
		t.Error("Custom complete callback not called")
	}
}

// mockSender needs RegisterPeer for the tests
func (m *mockSender) RegisterPeer(nodeID, dest string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sends == nil {
		m.sends = make(map[string][][]byte)
	}
	// We just track registrations by allowing sends to this nodeID
}
