package mesh

import (
	"context"
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
}

func newMockSender() *mockSender {
	return &mockSender{
		sends: make(map[string][][]byte),
	}
}

func (m *mockSender) SendTo(nodeID string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sends[nodeID] = append(m.sends[nodeID], data)
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
