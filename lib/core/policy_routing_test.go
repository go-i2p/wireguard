package core

import (
	"runtime"
	"strings"
	"testing"
)

// TestNewPlatformPolicyRoutingManager tests that platform-specific policy routing managers can be created.
func TestNewPlatformPolicyRoutingManager(t *testing.T) {
	manager, err := newPlatformPolicyRoutingManager()
	if err != nil {
		t.Fatalf("newPlatformPolicyRoutingManager() failed: %v", err)
	}

	if manager == nil {
		t.Error("newPlatformPolicyRoutingManager() returned nil manager")
	}

	// Verify the manager implements the interface
	if !manager.IsActive() {
		// Should start inactive - this is expected
		t.Logf("Policy routing manager starts inactive (expected)")
	}

	t.Logf("Created policy routing manager for %s", runtime.GOOS)
}

// TestPolicyRoutingManagerInterface tests the basic interface compliance.
func TestPolicyRoutingManagerInterface(t *testing.T) {
	manager, err := newPlatformPolicyRoutingManager()
	if err != nil {
		t.Fatalf("newPlatformPolicyRoutingManager() failed: %v", err)
	}

	// Test initial state
	if manager.IsActive() {
		t.Error("New policy routing manager should not be active initially")
	}

	// Test setup with empty parameters (should fail)
	err = manager.Setup("", "wg0")
	if err == nil {
		t.Error("Setup with empty upstream interface should fail")
	}

	err = manager.Setup("eth0", "")
	if err == nil {
		t.Error("Setup with empty mesh interface should fail")
	}

	// Test teardown when not active (should be safe)
	err = manager.Teardown()
	if err != nil {
		t.Errorf("Teardown when not active should be safe: %v", err)
	}
}

// TestExitNodePolicyRouting tests policy routing integration with ExitNode.
func TestExitNodePolicyRouting(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:          true,
		PublicInterface:  "eth0",
		AutoDetect:       false,
		FallbackBehavior: "direct",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() failed: %v", err)
	}

	// Test initial state
	if exitNode.IsPolicyRoutingActive() {
		t.Error("New exit node should not have policy routing active")
	}

	if exitNode.GetUpstreamInterface() != "" {
		t.Error("New exit node should have empty upstream interface")
	}

	// Test setup policy routing with non-existent interface (should handle gracefully)
	err = exitNode.SetupPolicyRouting("nonexistent999")
	// This should either work (with fallback) or fail gracefully depending on FallbackBehavior
	if err != nil {
		t.Logf("SetupPolicyRouting with non-existent interface failed as expected: %v", err)
	}

	// Test teardown (should be safe even if setup failed)
	err = exitNode.TeardownPolicyRouting()
	if err != nil {
		t.Errorf("TeardownPolicyRouting should be safe: %v", err)
	}
}

// TestExitNodeAutoDetection tests VPN auto-detection functionality.
func TestExitNodeAutoDetection(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:          true,
		PublicInterface:  "eth0",
		AutoDetect:       true,
		FallbackBehavior: "direct",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() failed: %v", err)
	}

	// Test auto-detection setup (should handle no VPNs gracefully)
	err = exitNode.SetupPolicyRouting("")
	if err != nil {
		t.Logf("Auto-detection setup failed (expected if no VPNs): %v", err)
	}

	// Should still be safe to teardown
	err = exitNode.TeardownPolicyRouting()
	if err != nil {
		t.Errorf("TeardownPolicyRouting should be safe: %v", err)
	}
}

// TestExitNodeBlockFallback tests the "block" fallback behavior.
func TestExitNodeBlockFallback(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:          true,
		PublicInterface:  "eth0",
		AutoDetect:       true,
		FallbackBehavior: "block",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() failed: %v", err)
	}

	// Test auto-detection with block fallback (should fail if no VPNs)
	err = exitNode.SetupPolicyRouting("")
	if err == nil {
		t.Log("Auto-detection setup succeeded (VPN must be available)")
	} else {
		t.Logf("Auto-detection setup failed with block fallback (expected): %v", err)
	}

	// Test explicit non-existent interface with block fallback
	err = exitNode.SetupPolicyRouting("nonexistent999")
	if err == nil {
		t.Error("Setup with non-existent interface and block fallback should fail")
	} else {
		t.Logf("Setup with non-existent interface failed as expected: %v", err)
	}
}

// TestConfigurationDefaults tests that configuration defaults are sensible.
func TestConfigurationDefaults(t *testing.T) {
	config := DefaultConfig()

	// Test exit node defaults
	if config.ExitNode.AutoDetect {
		t.Error("Auto-detect should be false by default")
	}

	if config.ExitNode.FallbackBehavior != "direct" {
		t.Errorf("Default fallback behavior should be 'direct', got %s", config.ExitNode.FallbackBehavior)
	}

	if config.ExitNode.UpstreamVPN != "" {
		t.Errorf("Default upstream VPN should be empty, got %s", config.ExitNode.UpstreamVPN)
	}
}

// TestPolicyRoutingManagerCreationAcrossPlatforms tests that each platform can create its manager.
// This is a compile-time test that ensures all platform stubs exist.
func TestPolicyRoutingManagerCreationAcrossPlatforms(t *testing.T) {
	platformTests := []struct {
		name        string
		constructor func() (PolicyRoutingManager, error)
	}{
		{"Linux", func() (PolicyRoutingManager, error) { return newLinuxPolicyRoutingManager() }},
		{"Darwin", func() (PolicyRoutingManager, error) { return newDarwinPolicyRoutingManager() }},
		{"Windows", func() (PolicyRoutingManager, error) { return newWindowsPolicyRoutingManager() }},
		{"BSD", func() (PolicyRoutingManager, error) { return newBSDPolicyRoutingManager() }},
	}

	for _, tt := range platformTests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := tt.constructor()

			// On the current platform, it should succeed
			if runtime.GOOS == strings.ToLower(tt.name) ||
				(tt.name == "BSD" && (runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd")) {
				if err != nil {
					t.Errorf("%s constructor should succeed on %s: %v", tt.name, runtime.GOOS, err)
				}
				if manager == nil {
					t.Errorf("%s constructor returned nil manager on %s", tt.name, runtime.GOOS)
				}
			} else {
				// On other platforms, it should return an error (stub)
				if err == nil {
					t.Errorf("%s constructor should fail on %s (cross-platform stub)", tt.name, runtime.GOOS)
				}
				if manager != nil {
					t.Errorf("%s constructor should return nil manager on %s (cross-platform stub)", tt.name, runtime.GOOS)
				}
			}
		})
	}
}

// Test helper function (would need strings import)
func init() {
	// This is just to test that the package compiles with all the required functions
}
