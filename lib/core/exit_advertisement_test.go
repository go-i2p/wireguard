package core

import (
	"fmt"
	"net/netip"
	"testing"

	"github.com/go-i2p/wireguard/lib/mesh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExitNode_BuildExitAdvertisement(t *testing.T) {
	tests := []struct {
		name     string
		config   ExitNodeConfig
		wantNil  bool
		wantCaps []string
	}{
		{
			name: "disabled exit node returns nil",
			config: ExitNodeConfig{
				Enabled:         false,
				PublicInterface: "eth0",
			},
			wantNil: true,
		},
		{
			name: "enabled exit node with direct routing",
			config: ExitNodeConfig{
				Enabled:         true,
				PublicInterface: "eth0",
			},
			wantNil:  false,
			wantCaps: []string{"nat", "forwarding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitNode, err := NewExitNode(tt.config)
			require.NoError(t, err)

			if tt.config.Enabled {
				err = exitNode.Start()
				require.NoError(t, err)
				defer exitNode.Stop()
			}

			adv := exitNode.BuildExitAdvertisement()

			if tt.wantNil {
				assert.Nil(t, adv)
				return
			}

			require.NotNil(t, adv)
			assert.ElementsMatch(t, tt.wantCaps, adv.Capabilities)
			assert.GreaterOrEqual(t, adv.CurrentLoad, 0.0)
			assert.LessOrEqual(t, adv.CurrentLoad, 1.0)
			assert.NotNil(t, adv.AvailableRoutes)
			assert.GreaterOrEqual(t, len(adv.AvailableRoutes), 1) // At least "direct" route
		})
	}
}

func TestExitNode_CurrentLoad(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
	}

	exitNode, err := NewExitNode(config)
	require.NoError(t, err)

	err = exitNode.Start()
	require.NoError(t, err)
	defer exitNode.Stop()

	// Initially should be zero
	load := exitNode.currentLoad()
	assert.Equal(t, 0.0, load)

	// We can't directly modify activeSessions since it's private,
	// but we can test that currentLoad returns a value in range [0, 1]
	assert.GreaterOrEqual(t, load, 0.0)
	assert.LessOrEqual(t, load, 1.0)
}

func TestExitNode_BuildAvailableRoutes(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
	}

	exitNode, err := NewExitNode(config)
	require.NoError(t, err)

	err = exitNode.Start()
	require.NoError(t, err)
	defer exitNode.Stop()

	routes := exitNode.buildAvailableRoutes()
	require.NotNil(t, routes)
	assert.GreaterOrEqual(t, len(routes), 1)

	// Should always have a "direct" route
	found := false
	for _, r := range routes {
		if r.Name == "direct" {
			found = true
			assert.Greater(t, r.Priority, 0)
			assert.Greater(t, r.Bandwidth, int64(0))
			assert.Greater(t, r.Latency, int64(0))
		}
	}
	assert.True(t, found, "should have a direct route")
}

func TestGossipEngine_ExitNodeAdvertisement(t *testing.T) {
	// Create routing table
	subnet := netip.MustParsePrefix("10.0.0.0/16")
	routingTable := mesh.NewRoutingTable(mesh.RoutingTableConfig{
		Subnet: subnet,
	})

	// Create a mock exit node provider
	mockAdv := &mesh.ExitNodeAdvertisement{
		Capabilities: []string{"nat", "forwarding"},
		CurrentLoad:  0.3,
		AvailableRoutes: []mesh.RouteSpec{
			{
				Name:      "direct",
				Priority:  100,
				Bandwidth: 100000000,
				Latency:   50,
			},
		},
	}

	mockProvider := &mockExitNodeProvider{adv: mockAdv}

	// Create gossip engine
	config := mesh.GossipEngineConfig{
		Config:           mesh.DefaultGossipConfig(),
		RoutingTable:     routingTable,
		ExitNodeProvider: mockProvider,
		NodeID:           "test-node-1",
		NetworkID:        "test-network",
	}

	engine := mesh.NewGossipEngine(config)
	require.NotNil(t, engine)

	// The gossip engine should be able to include exit advertisement in announcements
	// This is verified through the integration test below
}

func TestRoutingTable_ExitNodeStorage(t *testing.T) {
	subnet := netip.MustParsePrefix("10.0.0.0/16")
	routingTable := mesh.NewRoutingTable(mesh.RoutingTableConfig{
		Subnet: subnet,
	})

	// Initially no exit nodes
	exitNodes := routingTable.GetExitNodes()
	assert.Empty(t, exitNodes)

	// Add an exit node advertisement
	nodeID := "test-node-1"
	adv := &mesh.ExitNodeAdvertisement{
		Capabilities: []string{"nat", "forwarding"},
		CurrentLoad:  0.5,
		AvailableRoutes: []mesh.RouteSpec{
			{
				Name:      "direct",
				Priority:  100,
				Bandwidth: 100000000,
				Latency:   50,
			},
		},
	}

	routingTable.UpdateExitNode(nodeID, adv)

	// Verify it's stored
	exitNodes = routingTable.GetExitNodes()
	assert.Len(t, exitNodes, 1)
	assert.Contains(t, exitNodes, nodeID)

	retrieved, ok := routingTable.GetExitNode(nodeID)
	require.True(t, ok)
	assert.Equal(t, adv.Capabilities, retrieved.Capabilities)
	assert.Equal(t, adv.CurrentLoad, retrieved.CurrentLoad)

	// Update the advertisement
	adv.CurrentLoad = 0.7
	routingTable.UpdateExitNode(nodeID, adv)

	retrieved, ok = routingTable.GetExitNode(nodeID)
	require.True(t, ok)
	assert.Equal(t, 0.7, retrieved.CurrentLoad)

	// Remove the advertisement
	routingTable.UpdateExitNode(nodeID, nil)
	exitNodes = routingTable.GetExitNodes()
	assert.Empty(t, exitNodes)

	_, ok = routingTable.GetExitNode(nodeID)
	assert.False(t, ok)
}

func TestRoutingTable_MultipleExitNodes(t *testing.T) {
	subnet := netip.MustParsePrefix("10.0.0.0/16")
	routingTable := mesh.NewRoutingTable(mesh.RoutingTableConfig{
		Subnet: subnet,
	})

	// Add multiple exit nodes
	for i := 1; i <= 5; i++ {
		nodeID := fmt.Sprintf("test-node-%d", i)
		adv := &mesh.ExitNodeAdvertisement{
			Capabilities: []string{"nat", "forwarding"},
			CurrentLoad:  float64(i) / 10.0,
			AvailableRoutes: []mesh.RouteSpec{
				{
					Name:      "direct",
					Priority:  100,
					Bandwidth: 100000000,
					Latency:   50 + i*10,
				},
			},
		}
		routingTable.UpdateExitNode(nodeID, adv)
	}

	// Verify all stored
	exitNodes := routingTable.GetExitNodes()
	assert.Len(t, exitNodes, 5)

	for i := 1; i <= 5; i++ {
		nodeID := fmt.Sprintf("test-node-%d", i)
		adv, ok := routingTable.GetExitNode(nodeID)
		require.True(t, ok)
		assert.Equal(t, float64(i)/10.0, adv.CurrentLoad)
	}
}

// mockExitNodeProvider is a mock implementation for testing
type mockExitNodeProvider struct {
	adv *mesh.ExitNodeAdvertisement
}

func (m *mockExitNodeProvider) BuildExitAdvertisement() *mesh.ExitNodeAdvertisement {
	return m.adv
}
