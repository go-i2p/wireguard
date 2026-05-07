package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyUpstreamVPN_NoConfig tests health check with no upstream VPN configured
func TestVerifyUpstreamVPN_NoConfig(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         false,
		PublicInterface: "eth0",
		// No UpstreamVPN or AutoDetect
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		// On systems without iptables, NewExitNode may fail
		t.Skipf("Skipping test: %v", err)
	}

	// Should return nil when no upstream VPN is configured
	err = exitNode.VerifyUpstreamVPN()
	assert.NoError(t, err)
}

// TestVerifyUpstreamVPN_AutoDetect tests health check with auto-detect enabled
func TestVerifyUpstreamVPN_AutoDetect(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         false,
		PublicInterface: "eth0",
		AutoDetect:      true,
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Should not error even if no VPNs are found (returns error explaining none found)
	err = exitNode.VerifyUpstreamVPN()
	// This may error or not depending on whether VPNs are present
	// Just ensure it doesn't panic
	t.Logf("VerifyUpstreamVPN result: %v", err)
}

// TestVerifyUpstreamVPN_NonExistentInterface tests health check with invalid interface
func TestVerifyUpstreamVPN_NonExistentInterface(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         false,
		PublicInterface: "eth0",
		UpstreamVPN:     "nonexistent999",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Manually set the upstream interface for testing
	exitNode.mu.Lock()
	exitNode.upstreamInterface = "nonexistent999"
	exitNode.mu.Unlock()

	// Should return error for non-existent interface
	err = exitNode.VerifyUpstreamVPN()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestVerifyUpstreamVPN_LoopbackInterface tests health check with loopback (should pass)
func TestVerifyUpstreamVPN_LoopbackInterface(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         false,
		PublicInterface: "eth0",
		UpstreamVPN:     "lo",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Manually set the upstream interface to loopback (which should exist and be up)
	exitNode.mu.Lock()
	exitNode.upstreamInterface = "lo"
	exitNode.mu.Unlock()

	// Should pass for loopback interface
	err = exitNode.VerifyUpstreamVPN()
	assert.NoError(t, err)
}

// TestHealthCheckLoop_Integration tests the health check loop lifecycle
func TestHealthCheckLoop_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
		UpstreamVPN:     "lo", // Use loopback for testing
		AutoDetect:      false,
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Set upstream interface manually
	exitNode.mu.Lock()
	exitNode.upstreamInterface = "lo"
	exitNode.mu.Unlock()

	// Start the exit node (which starts health check loop)
	err = exitNode.Start()
	if err != nil {
		t.Skipf("Skipping test due to start failure: %v", err)
	}
	defer exitNode.Stop()

	// Wait a bit to let the health check run at least once
	time.Sleep(2 * time.Second)

	// If we got here without panic, the health check loop is working
	assert.True(t, exitNode.IsActive())
}

// TestBuildExitAdvertisement_AfterHealthCheck tests that advertisements reflect health status
func TestBuildExitAdvertisement_AfterHealthCheck(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
		UpstreamVPN:     "lo",
		AutoDetect:      false,
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Set upstream interface
	exitNode.mu.Lock()
	exitNode.upstreamInterface = "lo"
	exitNode.mu.Unlock()

	// Start to make it active
	err = exitNode.Start()
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}
	defer exitNode.Stop()

	// Run health check
	err = exitNode.VerifyUpstreamVPN()
	require.NoError(t, err)

	// Build advertisement
	adv := exitNode.BuildExitAdvertisement()
	require.NotNil(t, adv)

	// Should have capabilities
	assert.NotEmpty(t, adv.Capabilities)

	// Should have available routes
	assert.NotEmpty(t, adv.AvailableRoutes)
}
