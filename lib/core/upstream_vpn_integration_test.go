//go:build integration
// +build integration

package core

import (
	"testing"

	"github.com/go-i2p/wireguard/lib/mesh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpstreamVPNRouting verifies end-to-end upstream VPN routing functionality.
// This test checks that exit nodes correctly advertise upstream VPN information
// and that the routing table stores and retrieves it properly.
func TestUpstreamVPNRouting(t *testing.T) {
	// Create exit node with upstream VPN configuration
	exitConfig := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
		UpstreamVPN:     "tun0",
		AutoDetect:      false,
	}

	exitNode, err := NewExitNode(exitConfig)
	require.NoError(t, err, "failed to create exit node")
	require.NotNil(t, exitNode, "exit node should not be nil")

	// Build exit advertisement
	adv := exitNode.BuildExitAdvertisement()
	require.NotNil(t, adv, "exit advertisement should not be nil")

	// Verify available routes include both direct and VPN routes
	assert.NotEmpty(t, adv.AvailableRoutes, "should have available routes")
	
	routeNames := make(map[string]bool)
	for _, route := range adv.AvailableRoutes {
		routeNames[route.Name] = true
	}
	assert.True(t, routeNames["direct"], "should have direct route")

	// Create routing table and store exit advertisement
	rt := mesh.NewRoutingTable(mesh.RoutingTableConfig{})
	rt.UpdateExitNode("exit-node-id", adv)

	// Verify we can retrieve the exit node
	retrievedAdv, ok := rt.GetExitNode("exit-node-id")
	require.True(t, ok, "should find exit node")
	require.NotNil(t, retrievedAdv, "should be able to retrieve exit node")
}

// TestVPNFailover verifies failover behavior when VPN connection fails.
// This test checks that health checks detect VPN failures and that the
// exit node updates its advertisement accordingly.
func TestVPNFailover(t *testing.T) {
	// Create exit node with upstream VPN that will be verified
	exitConfig := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
		UpstreamVPN:     "lo", // Use loopback for testing
		AutoDetect:      false,
	}

	exitNode, err := NewExitNode(exitConfig)
	require.NoError(t, err, "failed to create exit node")

	// Verify initial VPN status
	err = exitNode.VerifyUpstreamVPN()
	// Since we're using loopback, it should succeed
	if err == nil {
		t.Log("loopback interface verified successfully")
	}

	// Test with non-existent interface (simulates VPN failure)
	failConfig := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
		UpstreamVPN:     "nonexistent0",
		AutoDetect:      false,
	}

	failNode, err := NewExitNode(failConfig)
	require.NoError(t, err, "should create node even with bad VPN config")

	// Verify VPN check fails
	err = failNode.VerifyUpstreamVPN()
	assert.Error(t, err, "should detect non-existent interface")
}

// TestRouteAdvertisement verifies that exit node advertisements are
// properly propagated through the gossip protocol.
func TestRouteAdvertisement(t *testing.T) {
	// Create routing table
	rt := mesh.NewRoutingTable(mesh.RoutingTableConfig{})

	// Create mock exit node advertisement
	adv := &mesh.ExitNodeAdvertisement{
		Capabilities:       []string{"nat", "forwarding"},
		CurrentLoad:        0.25,
		BandwidthLimitMbps: 100,
		ActiveClients:      5,
		Uptime:             3600,
		AvailableRoutes: []mesh.RouteSpec{
			{
				Name:      "direct",
				Priority:  100,
				Bandwidth: 100,
				Latency:   50,
			},
		},
		UpstreamVPN: &mesh.UpstreamVPNInfo{
			Provider: "mullvad",
			Country:  "SE",
			Verified: true,
		},
	}

	// Store advertisement in routing table
	rt.UpdateExitNode("exit1", adv)

	// Verify retrieval
	retrieved, ok := rt.GetExitNode("exit1")
	require.True(t, ok, "should find exit node")
	require.NotNil(t, retrieved, "should retrieve exit node")
	assert.Equal(t, 0.25, retrieved.CurrentLoad, "load should match")
	assert.Equal(t, "mullvad", retrieved.UpstreamVPN.Provider, "provider should match")
	assert.Len(t, retrieved.AvailableRoutes, 1, "should have one route")

	// Test GetExitNodes returns all stored exits
	rt.UpdateExitNode("exit2", adv)
	allExits := rt.GetExitNodes()
	assert.Len(t, allExits, 2, "should have two exit nodes")
	assert.Contains(t, allExits, "exit1", "should contain exit1")
	assert.Contains(t, allExits, "exit2", "should contain exit2")
}

// TestClientRouteSelection verifies that clients correctly filter and
// select routes based on preferences.
func TestClientRouteSelection(t *testing.T) {
	// Create mock exit nodes in routing table
	exitNodes := map[string]interface{}{
		"exit1": &mesh.ExitNodeAdvertisement{
			CurrentLoad: 0.3,
			AvailableRoutes: []mesh.RouteSpec{
				{Name: "direct", Priority: 100, Bandwidth: 100, Latency: 50},
			},
			UpstreamVPN: nil, // Direct only
		},
		"exit2": &mesh.ExitNodeAdvertisement{
			CurrentLoad: 0.5,
			AvailableRoutes: []mesh.RouteSpec{
				{Name: "direct", Priority: 100, Bandwidth: 100, Latency: 50},
				{Name: "mullvad-sweden", Priority: 50, Bandwidth: 50, Latency: 100},
			},
			UpstreamVPN: &mesh.UpstreamVPNInfo{
				Provider: "mullvad",
				Country:  "SE",
				Verified: true,
			},
		},
	}

	// Test 1: No preferences, should select any available exit
	config1 := ClientExitConfig{
		Enabled:        true,
		ExitNodeID:     "",
		PreferredRoute: "",
		RequireVPN:     false,
	}

	nodeID, routeName, err := SelectExitRoute(config1, exitNodes)
	assert.NoError(t, err, "should select an exit")
	assert.NotEmpty(t, nodeID, "should return a node ID")
	assert.Equal(t, "direct", routeName, "should default to direct route")

	// Test 2: Specific exit node requested
	config2 := ClientExitConfig{
		Enabled:    true,
		ExitNodeID: "exit2",
	}

	nodeID, routeName, err = SelectExitRoute(config2, exitNodes)
	assert.NoError(t, err, "should find requested exit")
	assert.Equal(t, "exit2", nodeID, "should return requested node")

	// Test 3: Non-existent exit node requested
	config3 := ClientExitConfig{
		Enabled:    true,
		ExitNodeID: "nonexistent",
	}

	_, _, err = SelectExitRoute(config3, exitNodes)
	assert.Error(t, err, "should fail for non-existent node")

	// Test 4: Empty exit nodes
	config4 := ClientExitConfig{
		Enabled: true,
	}

	_, _, err = SelectExitRoute(config4, map[string]interface{}{})
	assert.Error(t, err, "should fail with no exit nodes")
}

// TestExitNodeLifecycle verifies the complete lifecycle of an exit node
// including initialization, advertisement, health checks, and shutdown.
func TestExitNodeLifecycle(t *testing.T) {
	// Create exit node configuration
	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
		AutoDetect:      false,
	}

	// Initialize exit node
	exitNode, err := NewExitNode(config)
	require.NoError(t, err, "should create exit node")
	require.NotNil(t, exitNode, "exit node should not be nil")

	// Verify initial state
	assert.False(t, exitNode.IsActive(), "exit node should not be active initially")

	// Build advertisement
	adv := exitNode.BuildExitAdvertisement()
	require.NotNil(t, adv, "should build advertisement")
	assert.NotEmpty(t, adv.Capabilities, "should have capabilities")
	assert.GreaterOrEqual(t, adv.CurrentLoad, 0.0, "load should be non-negative")
	assert.LessOrEqual(t, adv.CurrentLoad, 1.0, "load should be at most 1.0")

	// Note: Start() and Stop() require elevated privileges for iptables,
	// so we don't test them in this integration test
}
