package core

import (
	"testing"
	"time"

	"github.com/go-i2p/wireguard/lib/mesh"
)

// TestNewExitDiscovery tests creation of exit discovery manager.
func TestNewExitDiscovery(t *testing.T) {
	tests := []struct {
		name       string
		staleAfter time.Duration
		wantStale  time.Duration
	}{
		{
			name:       "default stale duration",
			staleAfter: 0,
			wantStale:  5 * time.Minute,
		},
		{
			name:       "custom stale duration",
			staleAfter: 10 * time.Minute,
			wantStale:  10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := NewExitDiscovery(tt.staleAfter)

			if ed == nil {
				t.Fatal("NewExitDiscovery() returned nil")
			}

			if ed.staleAfter != tt.wantStale {
				t.Errorf("staleAfter = %v, want %v", ed.staleAfter, tt.wantStale)
			}

			if ed.exitNodes == nil {
				t.Error("exitNodes map not initialized")
			}
		})
	}
}

// TestUpdateExitNode tests updating exit node information.
func TestUpdateExitNode(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	announce := &mesh.PeerAnnounce{
		NodeID:    "node1",
		I2PDest:   "dest1",
		NetworkID: "test-net",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:             "node1",
			Capabilities:       []string{"ipv4", "ipv6"},
			BandwidthLimitMbps: 100,
			CurrentLoad:        0.3,
			ActiveClients:      5,
			Uptime:             1000,
		},
	}

	// Update with new exit node
	ed.UpdateExitNode(announce)

	info, err := ed.GetExitNode("node1")
	if err != nil {
		t.Fatalf("GetExitNode() error = %v", err)
	}

	if info.Advertisement.NodeID != "node1" {
		t.Errorf("NodeID = %s, want node1", info.Advertisement.NodeID)
	}

	if info.Advertisement.CurrentLoad != 0.3 {
		t.Errorf("CurrentLoad = %f, want 0.3", info.Advertisement.CurrentLoad)
	}

	// Update existing exit node with new info
	announce.ExitNode.CurrentLoad = 0.5
	ed.UpdateExitNode(announce)

	info, err = ed.GetExitNode("node1")
	if err != nil {
		t.Fatalf("GetExitNode() after update error = %v", err)
	}

	if info.Advertisement.CurrentLoad != 0.5 {
		t.Errorf("CurrentLoad after update = %f, want 0.5", info.Advertisement.CurrentLoad)
	}
}

// TestUpdateExitNode_NilCases tests nil handling.
func TestUpdateExitNode_NilCases(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	// Test with nil announce
	ed.UpdateExitNode(nil)

	// Test with announce but nil exit node
	announce := &mesh.PeerAnnounce{
		NodeID:    "node1",
		I2PDest:   "dest1",
		NetworkID: "test-net",
		ExitNode:  nil,
	}
	ed.UpdateExitNode(announce)

	// Should have no exit nodes
	nodes := ed.ListExitNodes()
	if len(nodes) != 0 {
		t.Errorf("ListExitNodes() after nil updates = %d, want 0", len(nodes))
	}
}

// TestRemoveExitNode tests removing exit nodes.
func TestRemoveExitNode(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	// Add exit node
	announce := &mesh.PeerAnnounce{
		NodeID:   "node1",
		ExitNode: &mesh.ExitNodeAdvertisement{NodeID: "node1"},
	}
	ed.UpdateExitNode(announce)

	// Verify it exists
	if _, err := ed.GetExitNode("node1"); err != nil {
		t.Fatalf("GetExitNode() before removal error = %v", err)
	}

	// Remove it
	ed.RemoveExitNode("node1")

	// Verify it's gone
	if _, err := ed.GetExitNode("node1"); err == nil {
		t.Error("GetExitNode() after removal should return error")
	}

	// Remove non-existent node (should not panic)
	ed.RemoveExitNode("nonexistent")
}

// TestCleanupStale tests stale entry cleanup.
func TestCleanupStale(t *testing.T) {
	ed := NewExitDiscovery(100 * time.Millisecond) // Short stale duration for testing

	// Add fresh exit node
	announce1 := &mesh.PeerAnnounce{
		NodeID:   "node1",
		ExitNode: &mesh.ExitNodeAdvertisement{NodeID: "node1"},
	}
	ed.UpdateExitNode(announce1)

	// Add another exit node
	announce2 := &mesh.PeerAnnounce{
		NodeID:   "node2",
		ExitNode: &mesh.ExitNodeAdvertisement{NodeID: "node2"},
	}
	ed.UpdateExitNode(announce2)

	// Wait for entries to become stale
	time.Sleep(150 * time.Millisecond)

	// Cleanup stale entries
	removed := ed.CleanupStale()

	if removed != 2 {
		t.Errorf("CleanupStale() removed = %d, want 2", removed)
	}

	nodes := ed.ListExitNodes()
	if len(nodes) != 0 {
		t.Errorf("ListExitNodes() after cleanup = %d, want 0", len(nodes))
	}
}

// TestDiscoverExitNodes tests discovery with filtering.
func TestDiscoverExitNodes(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	// Add multiple exit nodes
	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID: "node1",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:             "node1",
			Capabilities:       []string{"ipv4"},
			BandwidthLimitMbps: 100,
			CurrentLoad:        0.2,
			ActiveClients:      5,
		},
	})

	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID: "node2",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:             "node2",
			Capabilities:       []string{"ipv4", "ipv6"},
			BandwidthLimitMbps: 200,
			CurrentLoad:        0.5,
			ActiveClients:      10,
		},
	})

	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID: "node3",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:             "node3",
			Capabilities:       []string{"ipv4", "ipv6", "dns"},
			BandwidthLimitMbps: 0, // unlimited
			CurrentLoad:        0.1,
			ActiveClients:      2,
		},
	})

	tests := []struct {
		name      string
		filter    *ExitNodeFilter
		wantCount int
		wantFirst string // ID of first result (best node)
	}{
		{
			name:      "no filter - all nodes",
			filter:    nil,
			wantCount: 3,
			wantFirst: "node3", // lowest load
		},
		{
			name: "require ipv6",
			filter: &ExitNodeFilter{
				RequireCapabilities: []string{"ipv6"},
			},
			wantCount: 2,
			wantFirst: "node3",
		},
		{
			name: "require dns capability",
			filter: &ExitNodeFilter{
				RequireCapabilities: []string{"dns"},
			},
			wantCount: 1,
			wantFirst: "node3",
		},
		{
			name: "max load filter",
			filter: &ExitNodeFilter{
				MaxLoad: 0.3,
			},
			wantCount: 2,
			wantFirst: "node3",
		},
		{
			name: "min bandwidth filter",
			filter: &ExitNodeFilter{
				MinBandwidth: 150,
			},
			wantCount: 2, // node2 (200 Mbps) and node3 (unlimited)
			wantFirst: "node3",
		},
		{
			name: "exclude node filter",
			filter: &ExitNodeFilter{
				ExcludeNodeIDs: []string{"node1", "node2"},
			},
			wantCount: 1,
			wantFirst: "node3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := ed.DiscoverExitNodes(tt.filter)
			if err != nil {
				t.Fatalf("DiscoverExitNodes() error = %v", err)
			}

			if len(nodes) != tt.wantCount {
				t.Errorf("DiscoverExitNodes() count = %d, want %d", len(nodes), tt.wantCount)
			}

			if len(nodes) > 0 && nodes[0].Advertisement.NodeID != tt.wantFirst {
				t.Errorf("DiscoverExitNodes() first = %s, want %s", nodes[0].Advertisement.NodeID, tt.wantFirst)
			}
		})
	}
}

// TestDiscoverExitNodes_Empty tests discovery with no exit nodes.
func TestDiscoverExitNodes_Empty(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	_, err := ed.DiscoverExitNodes(nil)
	if err == nil {
		t.Error("DiscoverExitNodes() with empty cache should return error")
	}
}

// TestDiscoverExitNodes_NoMatches tests discovery with filter that matches nothing.
func TestDiscoverExitNodes_NoMatches(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID: "node1",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:       "node1",
			Capabilities: []string{"ipv4"},
		},
	})

	filter := &ExitNodeFilter{
		RequireCapabilities: []string{"nonexistent-capability"},
	}

	_, err := ed.DiscoverExitNodes(filter)
	if err == nil {
		t.Error("DiscoverExitNodes() with impossible filter should return error")
	}
}

// TestExitNodeFilter_Matches tests filter matching logic.
func TestExitNodeFilter_Matches(t *testing.T) {
	info := &ExitNodeInfo{
		Advertisement: &mesh.ExitNodeAdvertisement{
			NodeID:             "node1",
			Capabilities:       []string{"ipv4", "ipv6", "dns"},
			BandwidthLimitMbps: 100,
			CurrentLoad:        0.3,
			Location:           "US",
			AllowedClients:     []string{"client1", "client2"},
		},
	}

	tests := []struct {
		name   string
		filter *ExitNodeFilter
		want   bool
	}{
		{
			name:   "matches all capabilities",
			filter: &ExitNodeFilter{RequireCapabilities: []string{"ipv4", "ipv6"}},
			want:   true,
		},
		{
			name:   "missing capability",
			filter: &ExitNodeFilter{RequireCapabilities: []string{"ipv4", "nonexistent"}},
			want:   false,
		},
		{
			name:   "load below threshold",
			filter: &ExitNodeFilter{MaxLoad: 0.5},
			want:   true,
		},
		{
			name:   "load above threshold",
			filter: &ExitNodeFilter{MaxLoad: 0.2},
			want:   false,
		},
		{
			name:   "sufficient bandwidth",
			filter: &ExitNodeFilter{MinBandwidth: 50},
			want:   true,
		},
		{
			name:   "insufficient bandwidth",
			filter: &ExitNodeFilter{MinBandwidth: 200},
			want:   false,
		},
		{
			name:   "location matches",
			filter: &ExitNodeFilter{RequireLocation: "US"},
			want:   true,
		},
		{
			name:   "location doesn't match",
			filter: &ExitNodeFilter{RequireLocation: "EU"},
			want:   false,
		},
		{
			name:   "client allowed",
			filter: &ExitNodeFilter{CheckClientAllowed: "client1"},
			want:   true,
		},
		{
			name:   "client not allowed",
			filter: &ExitNodeFilter{CheckClientAllowed: "client3"},
			want:   false,
		},
		{
			name:   "node in allowed list",
			filter: &ExitNodeFilter{AllowedNodeIDs: []string{"node1", "node2"}},
			want:   true,
		},
		{
			name:   "node not in allowed list",
			filter: &ExitNodeFilter{AllowedNodeIDs: []string{"node2", "node3"}},
			want:   false,
		},
		{
			name:   "node not in exclude list",
			filter: &ExitNodeFilter{ExcludeNodeIDs: []string{"node2", "node3"}},
			want:   true,
		},
		{
			name:   "node in exclude list",
			filter: &ExitNodeFilter{ExcludeNodeIDs: []string{"node1", "node2"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Matches(info)
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExitNodeFilter_Matches_NilCases tests nil handling in filter matching.
func TestExitNodeFilter_Matches_NilCases(t *testing.T) {
	filter := &ExitNodeFilter{}

	// Nil info
	if filter.Matches(nil) {
		t.Error("Matches(nil) should return false")
	}

	// Nil advertisement
	info := &ExitNodeInfo{Advertisement: nil}
	if filter.Matches(info) {
		t.Error("Matches() with nil advertisement should return false")
	}
}

// TestSelectBestExitNode tests best exit node selection.
func TestSelectBestExitNode(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	// Add exit nodes with different loads
	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID: "node1",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:      "node1",
			CurrentLoad: 0.8,
		},
	})

	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID: "node2",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:      "node2",
			CurrentLoad: 0.3,
		},
	})

	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID: "node3",
		ExitNode: &mesh.ExitNodeAdvertisement{
			NodeID:      "node3",
			CurrentLoad: 0.1,
		},
	})

	best, err := ed.SelectBestExitNode(nil)
	if err != nil {
		t.Fatalf("SelectBestExitNode() error = %v", err)
	}

	if best.Advertisement.NodeID != "node3" {
		t.Errorf("SelectBestExitNode() = %s, want node3 (lowest load)", best.Advertisement.NodeID)
	}
}

// TestSelectBestExitNode_Empty tests selection with no exit nodes.
func TestSelectBestExitNode_Empty(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	_, err := ed.SelectBestExitNode(nil)
	if err == nil {
		t.Error("SelectBestExitNode() with empty cache should return error")
	}
}

// TestCalculateScore tests exit node scoring.
func TestCalculateScore(t *testing.T) {
	tests := []struct {
		name string
		info *ExitNodeInfo
		// We're checking relative scoring, not absolute values
		higherThan *ExitNodeInfo
	}{
		{
			name: "low load scores better than high load",
			info: &ExitNodeInfo{
				Advertisement: &mesh.ExitNodeAdvertisement{
					CurrentLoad:   0.1,
					ActiveClients: 5,
				},
			},
			higherThan: &ExitNodeInfo{
				Advertisement: &mesh.ExitNodeAdvertisement{
					CurrentLoad:   0.8,
					ActiveClients: 5,
				},
			},
		},
		{
			name: "fewer clients scores better",
			info: &ExitNodeInfo{
				Advertisement: &mesh.ExitNodeAdvertisement{
					CurrentLoad:   0.5,
					ActiveClients: 2,
				},
			},
			higherThan: &ExitNodeInfo{
				Advertisement: &mesh.ExitNodeAdvertisement{
					CurrentLoad:   0.5,
					ActiveClients: 20,
				},
			},
		},
		{
			name: "lower latency scores better",
			info: &ExitNodeInfo{
				Advertisement: &mesh.ExitNodeAdvertisement{
					CurrentLoad:   0.5,
					ActiveClients: 5,
				},
				Latency: 10 * time.Millisecond,
			},
			higherThan: &ExitNodeInfo{
				Advertisement: &mesh.ExitNodeAdvertisement{
					CurrentLoad:   0.5,
					ActiveClients: 5,
				},
				Latency: 200 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateScore(tt.info)
			higherScore := calculateScore(tt.higherThan)

			if score >= higherScore {
				t.Errorf("calculateScore() = %f should be less than %f", score, higherScore)
			}
		})
	}
}

// TestListExitNodes tests listing all exit nodes.
func TestListExitNodes(t *testing.T) {
	ed := NewExitDiscovery(5 * time.Minute)

	// Initially empty
	if len(ed.ListExitNodes()) != 0 {
		t.Error("ListExitNodes() should be empty initially")
	}

	// Add nodes
	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID:   "node1",
		ExitNode: &mesh.ExitNodeAdvertisement{NodeID: "node1"},
	})

	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID:   "node2",
		ExitNode: &mesh.ExitNodeAdvertisement{NodeID: "node2"},
	})

	nodes := ed.ListExitNodes()
	if len(nodes) != 2 {
		t.Errorf("ListExitNodes() count = %d, want 2", len(nodes))
	}
}

// TestGetExitNode_Stale tests getting stale exit node info.
func TestGetExitNode_Stale(t *testing.T) {
	ed := NewExitDiscovery(100 * time.Millisecond)

	ed.UpdateExitNode(&mesh.PeerAnnounce{
		NodeID:   "node1",
		ExitNode: &mesh.ExitNodeAdvertisement{NodeID: "node1"},
	})

	// Should be available immediately
	if _, err := ed.GetExitNode("node1"); err != nil {
		t.Fatalf("GetExitNode() fresh error = %v", err)
	}

	// Wait for staleness
	time.Sleep(150 * time.Millisecond)

	// Should now be stale
	_, err := ed.GetExitNode("node1")
	if err == nil {
		t.Error("GetExitNode() for stale entry should return error")
	}
}
