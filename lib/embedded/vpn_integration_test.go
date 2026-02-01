package embedded

import (
	"context"
	"testing"
	"time"
)

// TestVPN_Integration_FullLifecycle tests the complete VPN lifecycle with real I2P
func TestVPN_Integration_FullLifecycle(t *testing.T) {
	cfg := testConfig(t)
	cfg.TunnelLength = 1
	cfg.MaxPeers = 10
	cfg.EventBufferSize = 100

	vpn, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if vpn.State() != StateInitial {
		t.Errorf("Initial state = %s, want %s", vpn.State(), StateInitial)
	}

	// Start the VPN
	ctx := context.Background()
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if vpn.State() != StateRunning {
		t.Errorf("State after Start = %s, want %s", vpn.State(), StateRunning)
	}

	// Wait for I2P tunnel to initialize
	time.Sleep(5 * time.Second)

	// Test TunnelIP accessor - returns netip.Addr
	tunnelIP := vpn.TunnelIP()
	if !tunnelIP.IsValid() {
		t.Error("TunnelIP() returned invalid address after Start")
	} else {
		t.Logf("Tunnel IP: %s", tunnelIP.String())
	}

	// Test I2PDestination accessor
	i2pDest := vpn.I2PDestination()
	if i2pDest == "" {
		t.Error("I2PDestination() returned empty string after Start")
	}
	t.Logf("I2P Destination: %s", i2pDest)

	// Test I2PAddress accessor
	i2pAddr := vpn.I2PAddress()
	if i2pAddr == "" {
		t.Error("I2PAddress() returned empty string after Start")
	}
	t.Logf("I2P Address: %s", i2pAddr)

	// Test NodeID accessor
	nodeID := vpn.NodeID()
	if nodeID == "" {
		t.Error("NodeID() returned empty string after Start")
	}
	t.Logf("Node ID: %s", nodeID)

	// Test Node accessor
	node := vpn.Node()
	if node == nil {
		t.Error("Node() returned nil")
	}

	// Test Status - signature: Status() Status (no context parameter)
	status := vpn.Status()
	if status.NodeName != "integration-test-vpn" {
		t.Errorf("Status NodeName = %s, want integration-test-vpn", status.NodeName)
	}
	if status.State != StateRunning {
		t.Errorf("Status State = %s, want %s", status.State, StateRunning)
	}
	t.Logf("Status: %+v", status)

	// Test Config accessor
	config := vpn.Config()
	if config.NodeName != "integration-test-vpn" {
		t.Errorf("Config NodeName = %s, want integration-test-vpn", config.NodeName)
	}

	// Test Events channel
	events := vpn.Events()
	if events == nil {
		t.Error("Events() returned nil channel")
	}

	// Test DroppedEventCount
	droppedCount := vpn.DroppedEventCount()
	t.Logf("Dropped events: %d", droppedCount)

	// Test Done channel
	done := vpn.Done()
	if done == nil {
		t.Error("Done() returned nil channel")
	}

	// Stop the VPN properly
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := vpn.Stop(ctx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	defer cleanupVPN(t, vpn)

	if vpn.State() != StateStopped {
		t.Errorf("State after Close = %s, want %s", vpn.State(), StateStopped)
	}

	// Wait for done signal
	select {
	case <-done:
		t.Log("Done channel closed as expected")
	case <-time.After(2 * time.Second):
		t.Error("Done channel not closed after Close")
	}
}

// TestVPN_Integration_Peers tests peer management functionality
func TestVPN_Integration_Peers(t *testing.T) {
	cfg := testConfig(t)
	cfg.TunnelLength = 1

	vpn, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupVPN(t, vpn)

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// Test Peers method - signature: Peers() []PeerInfo (no context parameter)
	peers := vpn.Peers()
	t.Logf("Peers returned %d peers", len(peers))

	// Test PeerCount method
	count := vpn.PeerCount()
	if count != len(peers) {
		t.Errorf("PeerCount() = %d, but Peers() returned %d", count, len(peers))
	}
	t.Logf("Peer count: %d", count)

	// Test GetPeer with non-existent peer - signature: GetPeer(nodeID string) *PeerInfo
	peer := vpn.GetPeer("non-existent-peer")
	if peer != nil {
		t.Error("GetPeer with non-existent ID should return nil")
	}
}

// TestVPN_Integration_CreateInvite tests invite creation
func TestVPN_Integration_CreateInvite(t *testing.T) {
	cfg := testConfig(t)
	cfg.TunnelLength = 1

	vpn, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupVPN(t, vpn)

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// Test CreateInvite - signature: CreateInvite(expiry time.Duration, maxUses int) (string, error)
	inviteCode, err := vpn.CreateInvite(24*time.Hour, 5)
	if err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}
	if inviteCode == "" {
		t.Error("CreateInvite returned empty code")
	}
	t.Logf("Created invite: %s", inviteCode)

	// Test ListInvites - signature: ListInvites() []InviteInfo (no context, no bool parameter)
	invites := vpn.ListInvites()
	if len(invites) == 0 {
		t.Error("ListInvites returned 0 invites after creating one")
	}
	t.Logf("ListInvites returned %d invites", len(invites))

	// Verify the created invite is in the list
	found := false
	for _, inv := range invites {
		if inv.Code == inviteCode {
			found = true
			break
		}
	}
	if !found {
		t.Error("Created invite not found in ListInvites")
	}

	// Test RevokeInvite - signature: RevokeInvite(inviteCode string) error (no context)
	err = vpn.RevokeInvite(inviteCode)
	if err != nil {
		t.Errorf("RevokeInvite() error = %v", err)
	}

	// Verify invite is revoked
	invites = vpn.ListInvites()
	for _, inv := range invites {
		if inv.Code == inviteCode {
			t.Error("Revoked invite still appears in ListInvites")
		}
	}
}

// TestVPN_Integration_Routes tests route listing
func TestVPN_Integration_Routes(t *testing.T) {
	cfg := testConfig(t)
	cfg.TunnelLength = 1

	vpn, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupVPN(t, vpn)

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// Test Routes method
	routes := vpn.Routes()
	t.Logf("Routes returned %d routes", len(routes))

	// Test RouteCount method
	count := vpn.RouteCount()
	if count != len(routes) {
		t.Errorf("RouteCount() = %d, but Routes() returned %d", count, len(routes))
	}
	t.Logf("Route count: %d", count)
}

// TestVPN_Integration_Bans tests ban management
func TestVPN_Integration_Bans(t *testing.T) {
	cfg := testConfig(t)
	cfg.TunnelLength = 1

	vpn, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupVPN(t, vpn)

	// Wait for initialization
	time.Sleep(3 * time.Second)

	testNodeID := "bad-peer-456"

	// Test BanPeer - signature: BanPeer(nodeID, reason string, duration time.Duration) error
	err = vpn.BanPeer(testNodeID, "spam", 1*time.Hour)
	if err != nil {
		t.Fatalf("BanPeer() error = %v", err)
	}
	t.Logf("Banned peer: %s", testNodeID)

	// Access the underlying node to verify ban was added
	node := vpn.Node()
	if node == nil {
		t.Fatal("Node() returned nil")
	}

	bans := node.ListBans()
	t.Logf("ListBans returned %d bans", len(bans))

	// Verify the ban was added
	found := false
	for _, ban := range bans {
		if ban.NodeID == testNodeID {
			found = true
			if ban.Reason != "spam" {
				t.Errorf("Ban reason = %s, want spam", ban.Reason)
			}
			break
		}
	}
	if !found {
		t.Errorf("Ban for node %s not found", testNodeID)
	}

	// Test UnbanPeer - signature: UnbanPeer(nodeID string) error
	err = vpn.UnbanPeer(testNodeID)
	if err != nil {
		t.Errorf("UnbanPeer() error = %v", err)
	}

	// Verify the ban was removed
	bans = node.ListBans()
	for _, ban := range bans {
		if ban.NodeID == testNodeID {
			t.Errorf("Ban for node %s still exists after UnbanPeer", testNodeID)
		}
	}
}

// TestVPN_Integration_AcceptInvite tests accepting an invite
func TestVPN_Integration_AcceptInvite(t *testing.T) {
	// Create inviter VPN
	cfg1 := testConfig(t)
	cfg1.TunnelLength = 1

	vpn1, err := New(cfg1)
	if err != nil {
		t.Fatalf("New inviter failed: %v", err)
	}

	ctx := context.Background()
	if err := vpn1.Start(ctx); err != nil {
		t.Fatalf("Start inviter failed: %v", err)
	}
	defer cleanupVPN(t, vpn1)

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// Create invite
	inviteCode, err := vpn1.CreateInvite(24*time.Hour, 5)
	if err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}
	t.Logf("Created invite: %s", inviteCode)

	// Create invitee VPN
	cfg2 := testConfig(t)
	cfg2.TunnelLength = 1

	vpn2, err := New(cfg2)
	if err != nil {
		t.Fatalf("New invitee failed: %v", err)
	}

	if err := vpn2.Start(ctx); err != nil {
		t.Fatalf("Start invitee failed: %v", err)
	}
	defer cleanupVPN(t, vpn2)

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// Test AcceptInvite - signature: AcceptInvite(ctx context.Context, inviteCode string) error
	err = vpn2.AcceptInvite(ctx, inviteCode)
	if err != nil {
		t.Fatalf("AcceptInvite() error = %v", err)
	}

	t.Logf("Successfully accepted invite: %s", inviteCode)
}

// TestVPN_Integration_ConfigOptions tests various config options
func TestVPN_Integration_ConfigOptions(t *testing.T) {
	// Test WithRPC option - signature: WithRPC(enabled bool) Option
	cfg1 := testConfig(t)
	cfg1.EnableRPC = true
	cfg1.RPCSocket = "test-rpc.sock"

	vpn1, err := New(cfg1)
	if err != nil {
		t.Fatalf("New with RPC failed: %v", err)
	}

	ctx := context.Background()
	if err := vpn1.Start(ctx); err != nil {
		t.Fatalf("Start with RPC failed: %v", err)
	}
	vpn1.Close()

	// Test WithWeb option - signature: WithWeb(enabled bool, listenAddr string) Option
	cfg2 := testConfig(t)
	cfg2.EnableWeb = true
	cfg2.WebListenAddr = "127.0.0.1:18080"

	vpn2, err := New(cfg2)
	if err != nil {
		t.Fatalf("New with Web failed: %v", err)
	}

	if err := vpn2.Start(ctx); err != nil {
		t.Fatalf("Start with Web failed: %v", err)
	}
	defer cleanupVPN(t, vpn2)

	// Wait for web server to start
	time.Sleep(2 * time.Second)

	// Verify web is enabled
	config := vpn2.Config()
	if !config.EnableWeb {
		t.Error("Web should be enabled")
	}
	if config.WebListenAddr != "127.0.0.1:18080" {
		t.Errorf("Web address = %s, want 127.0.0.1:18080", config.WebListenAddr)
	}
}

// TestVPN_DefaultConfig tests DefaultConfig function
func TestVPN_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.NodeName == "" {
		t.Error("DefaultConfig NodeName is empty")
	}
	if cfg.DataDir == "" {
		t.Error("DefaultConfig DataDir is empty")
	}
	if cfg.SAMAddress != "127.0.0.1:7656" {
		t.Errorf("DefaultConfig SAMAddress = %s, want 127.0.0.1:7656", cfg.SAMAddress)
	}
	if cfg.TunnelSubnet != "10.42.0.0/16" {
		t.Errorf("DefaultConfig TunnelSubnet = %s, want 10.42.0.0/16", cfg.TunnelSubnet)
	}
	if cfg.TunnelLength != 1 {
		t.Errorf("DefaultConfig TunnelLength = %d, want 1", cfg.TunnelLength)
	}
	if cfg.MaxPeers != 50 {
		t.Errorf("DefaultConfig MaxPeers = %d, want 50", cfg.MaxPeers)
	}
	if cfg.EventBufferSize != 100 {
		t.Errorf("DefaultConfig EventBufferSize = %d, want 100", cfg.EventBufferSize)
	}
}
