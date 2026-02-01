package core

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestNode_Integration_CreateAndAcceptInvite tests the full invite workflow with real I2P
func TestNode_Integration_CreateAndAcceptInvite(t *testing.T) {
	samAddr := "127.0.0.1:7656"

	// Create first node (inviter)
	cfg1 := testConfig(t)
	cfg1.Node.Name = "node1"
	cfg1.I2P.SAMAddress = samAddr
	cfg1.RPC.Enabled = false
	cfg1.Web.Enabled = false

	node1, err := NewNode(cfg1)
	if err != nil {
		t.Fatalf("NewNode node1 failed: %v", err)
	}

	ctx := context.Background()
	if err := node1.Start(ctx); err != nil {
		t.Fatalf("Start node1 failed: %v", err)
	}
	defer cleanupNode(t, node1)

	// Wait for I2P tunnel to be ready
	time.Sleep(3 * time.Second)

	// Create invite
	result, err := node1.CreateInvite(24*time.Hour, 5)
	if err != nil {
		t.Fatalf("CreateInvite failed: %v", err)
	}

	if result.InviteCode == "" {
		t.Error("CreateInvite returned empty invite code")
	}
	if result.MaxUses != 5 {
		t.Errorf("CreateInvite MaxUses = %d, want 5", result.MaxUses)
	}

	t.Logf("Created invite: %s (max uses: %d)", result.InviteCode, result.MaxUses)

	// Create second node (invitee)
	cfg2 := testConfig(t)
	cfg2.Node.Name = "node2"
	cfg2.I2P.SAMAddress = samAddr
	cfg2.RPC.Enabled = false
	cfg2.Web.Enabled = false

	node2, err := NewNode(cfg2)
	if err != nil {
		t.Fatalf("NewNode node2 failed: %v", err)
	}

	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Start node2 failed: %v", err)
	}
	defer cleanupNode(t, node2)

	// Wait for I2P tunnel to be ready
	time.Sleep(3 * time.Second)

	// Accept invite
	acceptResult, err := node2.AcceptInvite(ctx, result.InviteCode)
	if err != nil {
		t.Fatalf("AcceptInvite failed: %v", err)
	}

	if acceptResult.NetworkID == "" {
		t.Error("AcceptInvite returned empty network ID")
	}
	if acceptResult.PeerNodeID == "" {
		t.Error("AcceptInvite returned empty peer node ID")
	}
	if acceptResult.TunnelIP == "" {
		t.Error("AcceptInvite returned empty tunnel IP")
	}

	t.Logf("Accepted invite: networkID=%s, peerNodeID=%s, tunnelIP=%s",
		acceptResult.NetworkID, acceptResult.PeerNodeID, acceptResult.TunnelIP)
}

// TestNode_Integration_ListRoutes tests route listing with real I2P
func TestNode_Integration_ListRoutes(t *testing.T) {
	samAddr := "127.0.0.1:7656"

	cfg := testConfig(t)
	cfg.Node.Name = "route-test-node"
	cfg.I2P.SAMAddress = samAddr
	cfg.RPC.Enabled = false
	cfg.Web.Enabled = false

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupNode(t, node)

	// Wait for initialization
	time.Sleep(2 * time.Second)

	// List routes
	routes := node.ListRoutes()
	if routes == nil {
		t.Error("ListRoutes returned nil")
	}

	t.Logf("ListRoutes returned %d routes", len(routes))
}

// TestNode_Integration_BanManagement tests ban/unban functionality
func TestNode_Integration_BanManagement(t *testing.T) {
	samAddr := "127.0.0.1:7656"

	cfg := testConfig(t)
	cfg.Node.Name = "ban-test-node"
	cfg.I2P.SAMAddress = samAddr
	cfg.RPC.Enabled = false
	cfg.Web.Enabled = false

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupNode(t, node)

	// Wait for initialization
	time.Sleep(2 * time.Second)

	// Add a ban
	testNodeID := "bad-node-123"
	testReason := "misbehavior"
	testDescription := "Test ban description"
	duration := 1 * time.Hour

	err = node.AddBan(testNodeID, testReason, testDescription, duration)
	if err != nil {
		t.Fatalf("AddBan failed: %v", err)
	}

	// List bans - signature: ListBans() []rpc.BanEntry
	bans := node.ListBans()
	if bans == nil {
		t.Fatal("ListBans returned nil")
	}

	found := false
	for _, ban := range bans {
		if ban.NodeID == testNodeID {
			found = true
			if ban.Reason != testReason {
				t.Errorf("Ban reason = %s, want %s", ban.Reason, testReason)
			}
			if ban.Description != testDescription {
				t.Errorf("Ban description = %s, want %s", ban.Description, testDescription)
			}
			break
		}
	}

	if !found {
		t.Errorf("Ban for node %s not found in ListBans", testNodeID)
	}

	t.Logf("ListBans returned %d bans", len(bans))

	// Remove ban - signature: RemoveBan(nodeID string) bool
	removed := node.RemoveBan(testNodeID)
	if !removed {
		t.Error("RemoveBan returned false, expected true")
	}

	// Verify ban is removed
	bans = node.ListBans()
	for _, ban := range bans {
		if ban.NodeID == testNodeID {
			t.Errorf("Ban for node %s still exists after RemoveBan", testNodeID)
		}
	}
}

// TestNode_Integration_AccessorMethods tests various getter methods
func TestNode_Integration_AccessorMethods(t *testing.T) {
	samAddr := "127.0.0.1:7656"

	cfg := testConfig(t)
	cfg.Node.Name = "accessor-test-node"
	cfg.I2P.SAMAddress = samAddr
	cfg.RPC.Enabled = false
	cfg.Web.Enabled = false

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupNode(t, node)

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// Test Identity accessor
	identity := node.Identity()
	if identity == nil {
		t.Error("Identity() returned nil")
	}

	// Test Transport accessor
	transport := node.Transport()
	if transport == nil {
		t.Error("Transport() returned nil")
	}

	// Test Device accessor
	device := node.Device()
	if device == nil {
		t.Error("Device() returned nil")
	}

	// Test RoutingTable accessor
	routingTable := node.RoutingTable()
	if routingTable == nil {
		t.Error("RoutingTable() returned nil")
	}

	// Test PeerManager accessor
	peerManager := node.PeerManager()
	if peerManager == nil {
		t.Error("PeerManager() returned nil")
	}

	// Test InviteStore accessor
	inviteStore := node.InviteStore()
	if inviteStore == nil {
		t.Error("InviteStore() returned nil")
	}

	// Test TunnelIPAddr accessor - returns netip.Addr
	tunnelIP := node.TunnelIPAddr()
	if !tunnelIP.IsValid() {
		t.Error("TunnelIPAddr() returned invalid address")
	} else {
		t.Logf("Tunnel IP: %s", tunnelIP.String())
	}
}

// TestNode_Integration_ConfigManagement tests dynamic configuration
func TestNode_Integration_ConfigManagement(t *testing.T) {
	samAddr := "127.0.0.1:7656"

	cfg := testConfig(t)
	cfg.Node.Name = "config-test-node"
	cfg.I2P.SAMAddress = samAddr
	cfg.RPC.Enabled = false
	cfg.Web.Enabled = false

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupNode(t, node)

	// Test GetConfig for various keys
	testCases := []struct {
		key      string
		expected interface{}
	}{
		{"node.name", "config-test-node"},
	}

	for _, tc := range testCases {
		value, err := node.GetConfig(tc.key)
		if err != nil {
			t.Errorf("GetConfig(%s) error = %v", tc.key, err)
			continue
		}
		if fmt.Sprintf("%v", value) != fmt.Sprintf("%v", tc.expected) {
			t.Errorf("GetConfig(%s) = %v, want %v", tc.key, value, tc.expected)
		}
	}

	// Test SetConfig for RPC TCP address (which is allowed at runtime)
	_, err = node.SetConfig("rpc.tcp_address", "127.0.0.1:9999")
	if err != nil {
		t.Errorf("SetConfig(rpc.tcp_address) error = %v", err)
	}

	// Verify the change
	value, err := node.GetConfig("rpc.tcp_address")
	if err != nil {
		t.Errorf("GetConfig(rpc.tcp_address) after set error = %v", err)
	}
	if value != "127.0.0.1:9999" {
		t.Errorf("GetConfig(rpc.tcp_address) = %v, want 127.0.0.1:9999", value)
	}
}

// TestNode_Integration_ListPeers tests peer listing
func TestNode_Integration_ListPeers(t *testing.T) {
	samAddr := "127.0.0.1:7656"

	cfg := testConfig(t)
	cfg.Node.Name = "peers-test-node"
	cfg.I2P.SAMAddress = samAddr
	cfg.RPC.Enabled = false
	cfg.Web.Enabled = false

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cleanupNode(t, node)

	// Wait for initialization
	time.Sleep(2 * time.Second)

	// List peers
	peers := node.ListPeers()
	if peers == nil {
		t.Error("ListPeers returned nil")
	}

	t.Logf("ListPeers returned %d peers", len(peers))

	// Verify peer count matches
	if node.PeerCount() != len(peers) {
		t.Errorf("PeerCount() = %d, but ListPeers returned %d", node.PeerCount(), len(peers))
	}
}
