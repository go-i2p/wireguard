// Package transport provides integration tests that require a real I2P router.
// These tests connect to SAM on localhost:7656 and will FAIL if the router is unavailable.
// No build tags or skip conditions - SAM availability is a hard requirement.
package transport

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-i2p/wireguard/i2pbind"
)

const (
	// samAddress is the address of the SAM bridge for integration tests.
	// Tests will fail if this is not available.
	samAddress = "127.0.0.1:7656"

	// testTimeout is the maximum time to wait for I2P operations.
	testTimeout = 60 * time.Second

	// dialTimeout is the timeout for initial SAM connection check.
	dialTimeout = 5 * time.Second
)

// requireSAM verifies SAM is available and fails the test if not.
// This is called at the start of each integration test.
func requireSAM(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", samAddress, dialTimeout)
	if err != nil {
		t.Fatalf("SAM bridge unavailable at %s: %v\n"+
			"Integration tests require a running I2P router with SAM enabled.\n"+
			"Start your I2P router and ensure SAM is listening on port 7656.",
			samAddress, err)
	}
	conn.Close()
}

// TestIntegration_TransportOpenClose tests opening and closing a transport with real I2P.
func TestIntegration_TransportOpenClose(t *testing.T) {
	requireSAM(t)

	tr := NewTransport("integration-test-open", samAddress, []string{
		"inbound.length=1",
		"outbound.length=1",
		"inbound.quantity=1",
		"outbound.quantity=1",
	})

	// Open the transport
	err := tr.Open()
	if err != nil {
		t.Fatalf("Failed to open transport: %v", err)
	}

	// Verify bind is accessible
	bind := tr.Bind()
	if bind == nil {
		t.Fatal("Bind() returned nil after Open()")
	}

	// Verify transport is open
	if !tr.IsOpen() {
		t.Error("Transport should be open after Open()")
	}

	// Verify we have a local I2P address
	localAddr := tr.LocalAddress()
	if localAddr == "" {
		t.Error("LocalAddress should not be empty after Open()")
	}
	t.Logf("Local I2P address: %s", localAddr)

	// Verify address looks like an I2P destination
	if !strings.HasSuffix(localAddr, ".i2p") && len(localAddr) < 50 {
		t.Errorf("LocalAddress doesn't look like I2P destination: %s", localAddr)
	}

	// Verify LocalDestination works
	dest, err := tr.LocalDestination()
	if err != nil {
		t.Fatalf("LocalDestination failed: %v", err)
	}
	if dest == "" {
		t.Error("LocalDestination returned empty address")
	}

	// Close the transport
	err = tr.Close()
	if err != nil {
		t.Errorf("Failed to close transport: %v", err)
	}

	// Verify transport is closed
	if tr.IsOpen() {
		t.Error("Transport should be closed after Close()")
	}

	// LocalAddress should be empty after close
	if tr.LocalAddress() != "" {
		t.Error("LocalAddress should be empty after Close()")
	}
}

// TestIntegration_TransportDoubleOpen tests that opening twice returns error.
func TestIntegration_TransportDoubleOpen(t *testing.T) {
	requireSAM(t)

	tr := NewTransport("integration-test-double", samAddress, []string{
		"inbound.length=1",
		"outbound.length=1",
	})

	// First open should succeed
	err := tr.Open()
	if err != nil {
		t.Fatalf("First Open failed: %v", err)
	}
	defer tr.Close()

	// Second open should fail
	err = tr.Open()
	if err == nil {
		t.Error("Second Open should return error")
	}
}

// TestIntegration_I2PBindOpenClose tests the i2pbind package directly.
func TestIntegration_I2PBindOpenClose(t *testing.T) {
	requireSAM(t)

	bind := i2pbind.NewI2PBindWithOptions("integration-bind-test", samAddress, []string{
		"inbound.length=1",
		"outbound.length=1",
		"inbound.quantity=1",
		"outbound.quantity=1",
	})

	// Open the bind
	recvFuncs, port, err := bind.Open(0)
	if err != nil {
		t.Fatalf("Failed to open bind: %v", err)
	}

	// Verify receive functions were returned
	if len(recvFuncs) == 0 {
		t.Error("Open should return at least one receive function")
	}
	t.Logf("Opened on port: %d, receive functions: %d", port, len(recvFuncs))

	// Verify we can get the local address
	localAddr, err := bind.LocalAddress()
	if err != nil {
		t.Fatalf("LocalAddress failed: %v", err)
	}
	if localAddr == "" {
		t.Error("LocalAddress returned empty string")
	}
	t.Logf("Local I2P address (base32): %s", localAddr)

	// Verify we can get the full destination
	dest, err := bind.LocalDestination()
	if err != nil {
		t.Fatalf("LocalDestination failed: %v", err)
	}
	if dest == "" {
		t.Error("LocalDestination returned empty address")
	}
	t.Logf("Local I2P destination length: %d", len(dest))

	// Verify BatchSize is 1 (I2P doesn't support batching)
	if bind.BatchSize() != 1 {
		t.Errorf("BatchSize should be 1, got %d", bind.BatchSize())
	}

	// Close the bind
	err = bind.Close()
	if err != nil {
		t.Errorf("Failed to close bind: %v", err)
	}

	// Double close should not error
	err = bind.Close()
	if err != nil {
		t.Errorf("Double close should not error: %v", err)
	}
}

// TestIntegration_I2PBindParseEndpoint tests parsing I2P endpoints.
func TestIntegration_I2PBindParseEndpoint(t *testing.T) {
	requireSAM(t)

	bind := i2pbind.NewI2PBindWithOptions("integration-endpoint-test", samAddress, []string{
		"inbound.length=1",
		"outbound.length=1",
	})

	// Open the bind first
	_, _, err := bind.Open(0)
	if err != nil {
		t.Fatalf("Failed to open bind: %v", err)
	}
	defer bind.Close()

	// Get our full destination to parse as an endpoint
	// ParseEndpoint requires full base64 destination, not base32 address
	localDest, err := bind.LocalDestination()
	if err != nil {
		t.Fatalf("LocalDestination failed: %v", err)
	}

	// Parse our own destination as an endpoint
	endpoint, err := bind.ParseEndpoint(string(localDest))
	if err != nil {
		t.Fatalf("ParseEndpoint failed: %v", err)
	}

	if endpoint == nil {
		t.Fatal("ParseEndpoint returned nil endpoint")
	}

	// Verify the endpoint destination matches
	dstStr := endpoint.DstToString()
	if dstStr == "" {
		t.Error("Endpoint DstToString returned empty string")
	}
	t.Logf("Parsed endpoint destination: %s", dstStr)
}

// TestIntegration_HealthMonitorWithRealSAM tests health monitoring with real SAM.
func TestIntegration_HealthMonitorWithRealSAM(t *testing.T) {
	requireSAM(t)

	cfg := HealthConfig{
		CheckInterval: 500 * time.Millisecond,
		Timeout:       2 * time.Second,
		RetryDelay:    1 * time.Second,
	}

	hm := NewHealthMonitor(samAddress, cfg)

	// Start the health monitor
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := hm.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start health monitor: %v", err)
	}
	defer hm.Stop()

	// Wait for at least one health check
	time.Sleep(1 * time.Second)

	// Should be healthy since SAM is available
	state := hm.State()
	if state != HealthStateHealthy {
		t.Errorf("Expected HealthStateHealthy, got %s", state)
	}

	if !hm.IsHealthy() {
		t.Error("IsHealthy should return true when SAM is available")
	}

	if hm.ConsecutiveFailures() != 0 {
		t.Errorf("Expected 0 consecutive failures, got %d", hm.ConsecutiveFailures())
	}
}

// TestIntegration_TransportPeerTracking tests peer tracking with real transport.
func TestIntegration_TransportPeerTracking(t *testing.T) {
	requireSAM(t)

	tr := NewTransport("integration-peer-test", samAddress, []string{
		"inbound.length=1",
		"outbound.length=1",
	})

	// Open the transport
	err := tr.Open()
	if err != nil {
		t.Fatalf("Failed to open transport: %v", err)
	}
	defer tr.Close()

	// Get our local address to use as a fake peer (for testing peer tracking)
	localAddr := tr.LocalAddress()
	if localAddr == "" {
		t.Fatal("LocalAddress is empty")
	}

	// Add a peer using our own address (just for tracking test)
	err = tr.AddPeer(localAddr, "testpubkey123")
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Verify peer was added
	if tr.PeerCount() != 1 {
		t.Errorf("Expected 1 peer, got %d", tr.PeerCount())
	}

	// Get the peer
	peer, ok := tr.GetPeer(localAddr)
	if !ok {
		t.Fatal("GetPeer should find the added peer")
	}

	if peer.I2PDest != localAddr {
		t.Errorf("Peer I2PDest mismatch: got %q", peer.I2PDest)
	}

	if peer.WGPublicKey != "testpubkey123" {
		t.Errorf("Peer WGPublicKey mismatch: got %q", peer.WGPublicKey)
	}

	if peer.State != PeerStateConnecting {
		t.Errorf("Initial peer state should be Connecting, got %s", peer.State)
	}

	// Update peer state
	tr.UpdatePeerState(localAddr, PeerStateConnected)
	peer, _ = tr.GetPeer(localAddr)
	if peer.State != PeerStateConnected {
		t.Errorf("Peer state should be Connected, got %s", peer.State)
	}

	// Verify connected count
	if tr.ConnectedPeerCount() != 1 {
		t.Errorf("Expected 1 connected peer, got %d", tr.ConnectedPeerCount())
	}

	// List peers
	peers := tr.ListPeers()
	if len(peers) != 1 {
		t.Errorf("Expected 1 peer in list, got %d", len(peers))
	}

	// Get full destination for ParseEndpoint (requires base64, not base32)
	localDest, err := tr.LocalDestination()
	if err != nil {
		t.Fatalf("LocalDestination failed: %v", err)
	}

	// Parse endpoint for the peer using full destination
	endpoint, err := tr.ParseEndpoint(string(localDest))
	if err != nil {
		t.Fatalf("ParseEndpoint failed: %v", err)
	}

	// Set the endpoint
	tr.SetPeerEndpoint(localAddr, endpoint)
	peer, _ = tr.GetPeer(localAddr)
	if peer.Endpoint == nil {
		t.Error("Peer endpoint should be set")
	}

	// Remove peer
	tr.RemovePeer(localAddr)
	if tr.PeerCount() != 0 {
		t.Errorf("Expected 0 peers after remove, got %d", tr.PeerCount())
	}
}

// TestIntegration_MultipleSessions tests creating multiple I2P sessions.
func TestIntegration_MultipleSessions(t *testing.T) {
	requireSAM(t)

	opts := []string{
		"inbound.length=1",
		"outbound.length=1",
		"inbound.quantity=1",
		"outbound.quantity=1",
	}

	// Create two transports with different tunnel names
	tr1 := NewTransport("integration-multi-1", samAddress, opts)
	tr2 := NewTransport("integration-multi-2", samAddress, opts)

	// Open first transport
	err := tr1.Open()
	if err != nil {
		t.Fatalf("Failed to open transport 1: %v", err)
	}
	defer tr1.Close()

	addr1 := tr1.LocalAddress()
	t.Logf("Transport 1 address: %s", addr1)

	// Open second transport
	err = tr2.Open()
	if err != nil {
		t.Fatalf("Failed to open transport 2: %v", err)
	}
	defer tr2.Close()

	addr2 := tr2.LocalAddress()
	t.Logf("Transport 2 address: %s", addr2)

	// Addresses should be different
	if addr1 == addr2 {
		t.Error("Two different sessions should have different I2P addresses")
	}

	// Both should be open
	if !tr1.IsOpen() {
		t.Error("Transport 1 should be open")
	}
	if !tr2.IsOpen() {
		t.Error("Transport 2 should be open")
	}
}

// TestIntegration_TransportWithCustomOptions tests custom SAM options.
func TestIntegration_TransportWithCustomOptions(t *testing.T) {
	requireSAM(t)

	// Test with minimal tunnel settings for faster tests
	opts := []string{
		"inbound.length=0",
		"outbound.length=0",
		"inbound.quantity=1",
		"outbound.quantity=1",
	}

	tr := NewTransport("integration-custom-opts", samAddress, opts)

	err := tr.Open()
	if err != nil {
		t.Fatalf("Failed to open transport with custom options: %v", err)
	}
	defer tr.Close()

	if !tr.IsOpen() {
		t.Error("Transport should be open")
	}

	localAddr := tr.LocalAddress()
	if localAddr == "" {
		t.Error("LocalAddress should not be empty")
	}
	t.Logf("Opened with zero-hop tunnels, address: %s", localAddr)
}
