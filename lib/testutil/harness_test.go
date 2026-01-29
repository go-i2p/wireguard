package testutil

import (
	"context"
	"testing"
	"time"
)

// requireSAM verifies SAM is available and fails the test if not.
func requireSAM(t *testing.T) {
	t.Helper()
	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	if err := harness.RequireSAM(); err != nil {
		t.Fatal(err)
	}
}

func TestMultiNodeHarness_RequireSAM(t *testing.T) {
	requireSAM(t)
	// If we get here, SAM is available
	t.Log("SAM is available at", DefaultSAMAddress)
}

func TestMultiNodeHarness_CreateSingleNode(t *testing.T) {
	requireSAM(t)

	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	defer harness.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	node, err := harness.CreateNode(ctx, "test1")
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Verify node is running
	if !node.IsStarted() {
		t.Error("Node should be started")
	}

	// Verify node has an address
	addr := node.LocalAddress()
	if addr == "" {
		t.Error("Node should have an address")
	}
	t.Logf("Node address: %s", addr)

	// Verify destination
	dest := node.I2PDestination()
	if dest == "" {
		t.Error("Node should have a destination")
	}
	if len(dest) < 500 {
		t.Errorf("Destination seems too short: %d chars", len(dest))
	}

	// Verify harness tracking
	if harness.NodeCount() != 1 {
		t.Errorf("Expected 1 node, got %d", harness.NodeCount())
	}

	// Verify GetNode
	retrieved := harness.GetNode("test1")
	if retrieved != node {
		t.Error("GetNode should return the same node")
	}
}

func TestMultiNodeHarness_CreateMultipleNodes(t *testing.T) {
	requireSAM(t)

	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	defer harness.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create 3 nodes
	nodes, err := harness.CreateNodes(ctx, 3)
	if err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}

	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodes))
	}

	// Verify all nodes have unique addresses
	addrs := make(map[string]bool)
	for _, node := range nodes {
		addr := node.LocalAddress()
		if addr == "" {
			t.Errorf("Node %s has no address", node.ID)
			continue
		}
		if addrs[addr] {
			t.Errorf("Duplicate address found: %s", addr)
		}
		addrs[addr] = true
		t.Logf("Node %s: %s", node.ID, addr)
	}

	// Verify harness tracking
	if harness.NodeCount() != 3 {
		t.Errorf("Expected 3 nodes in harness, got %d", harness.NodeCount())
	}

	// Verify ListNodes returns all nodes
	listed := harness.ListNodes()
	if len(listed) != 3 {
		t.Errorf("ListNodes should return 3 nodes, got %d", len(listed))
	}

	// Verify GetNodeAddresses
	addrMap := harness.GetNodeAddresses()
	if len(addrMap) != 3 {
		t.Errorf("GetNodeAddresses should return 3 entries, got %d", len(addrMap))
	}
}

func TestMultiNodeHarness_ConnectPeers(t *testing.T) {
	requireSAM(t)

	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	defer harness.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create 2 nodes
	nodes, err := harness.CreateNodes(ctx, 2)
	if err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}

	node1, node2 := nodes[0], nodes[1]
	t.Logf("Node1: %s", node1.LocalAddress())
	t.Logf("Node2: %s", node2.LocalAddress())

	// Connect the peers
	err = harness.ConnectPeers("node1", "node2")
	if err != nil {
		t.Fatalf("Failed to connect peers: %v", err)
	}

	// Verify peer tracking in node1
	peer2, ok := node1.Transport.GetPeer(node2.LocalAddress())
	if !ok {
		t.Error("Node1 should have node2 as peer")
	} else {
		t.Logf("Node1 sees peer: %s", peer2.I2PDest)
	}

	// Verify peer tracking in node2
	peer1, ok := node2.Transport.GetPeer(node1.LocalAddress())
	if !ok {
		t.Error("Node2 should have node1 as peer")
	} else {
		t.Logf("Node2 sees peer: %s", peer1.I2PDest)
	}
}

func TestMultiNodeHarness_DuplicateNodeID(t *testing.T) {
	requireSAM(t)

	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	defer harness.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create first node
	_, err := harness.CreateNode(ctx, "dup")
	if err != nil {
		t.Fatalf("Failed to create first node: %v", err)
	}

	// Try to create duplicate
	_, err = harness.CreateNode(ctx, "dup")
	if err == nil {
		t.Error("Creating duplicate node should fail")
	}
}

func TestMultiNodeHarness_ConnectNonexistentPeers(t *testing.T) {
	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	defer harness.Cleanup()

	// Try to connect non-existent nodes
	err := harness.ConnectPeers("nonexistent1", "nonexistent2")
	if err == nil {
		t.Error("Connecting non-existent peers should fail")
	}
}

func TestMultiNodeHarness_CleanupIdempotent(t *testing.T) {
	requireSAM(t)

	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a node
	_, err := harness.CreateNode(ctx, "cleanup-test")
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Cleanup should work
	harness.Cleanup()

	// Second cleanup should not panic
	harness.Cleanup()

	// Harness should be empty
	if harness.NodeCount() != 0 {
		t.Error("Harness should be empty after cleanup")
	}
}

func TestTestNode_DoubleStart(t *testing.T) {
	requireSAM(t)

	node := NewTestNode(TestNodeConfig{
		ID: "double-start",
	})
	defer node.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First start should succeed
	err := node.Start(ctx)
	if err != nil {
		t.Fatalf("First start failed: %v", err)
	}

	// Second start should fail
	err = node.Start(ctx)
	if err == nil {
		t.Error("Second start should fail")
	}
}

func TestTestNode_StopWithoutStart(t *testing.T) {
	node := NewTestNode(TestNodeConfig{
		ID: "never-started",
	})

	// Stop without start should not error
	err := node.Stop()
	if err != nil {
		t.Errorf("Stop without start should not error: %v", err)
	}
}

func TestMultiNodeHarness_MeshTopology(t *testing.T) {
	requireSAM(t)

	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	defer harness.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Create 4 nodes
	nodes, err := harness.CreateNodes(ctx, 4)
	if err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}

	t.Log("Created 4 nodes:")
	for _, n := range nodes {
		t.Logf("  %s: %s", n.ID, n.LocalAddress())
	}

	// Create a mesh: connect each node to every other node
	connections := 0
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			nodeA := nodes[i].ID
			nodeB := nodes[j].ID
			if err := harness.ConnectPeers(nodeA, nodeB); err != nil {
				t.Errorf("Failed to connect %s to %s: %v", nodeA, nodeB, err)
			} else {
				connections++
			}
		}
	}

	t.Logf("Created %d peer connections", connections)

	// Verify each node has 3 peers (connected to all others)
	for _, node := range nodes {
		peerCount := node.Transport.PeerCount()
		if peerCount != 3 {
			t.Errorf("Node %s should have 3 peers, got %d", node.ID, peerCount)
		}
	}
}

func TestMultiNodeHarness_WaitForNodes(t *testing.T) {
	requireSAM(t)

	harness := NewMultiNodeHarness(MultiNodeHarnessConfig{})
	defer harness.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create nodes
	_, err := harness.CreateNodes(ctx, 2)
	if err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}

	// WaitForNodes should succeed since nodes are created
	err = harness.WaitForNodes(ctx)
	if err != nil {
		t.Errorf("WaitForNodes failed: %v", err)
	}
}
