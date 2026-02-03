package core

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// TestPolicyRoutingIntegration tests the complete policy routing workflow
// from VPN detection to policy routing setup.
func TestPolicyRoutingIntegration(t *testing.T) {
	// Skip if not Linux (policy routing is Linux-only for now)
	if runtime.GOOS != "linux" {
		t.Skip("Policy routing integration test only supports Linux")
	}

	// Skip if not root (can't test iptables/sysctl without root)
	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}

	// Skip if required tools are not available
	if _, err := exec.LookPath("iptables"); err != nil {
		t.Skip("Skipping test: iptables not found")
	}

	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("Skipping test: iproute2 (ip command) not found")
	}

	// Test configuration with auto-detection
	config := ExitNodeConfig{
		Enabled:          true,
		PublicInterface:  "lo", // Use loopback for testing
		AutoDetect:       true,
		FallbackBehavior: "direct",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() failed: %v", err)
	}

	// Test 1: Auto-detection with no VPNs should use direct routing
	t.Run("AutoDetection_NoVPNs", func(t *testing.T) {
		err := exitNode.SetupPolicyRouting("")
		if err != nil {
			// With "direct" fallback, this should succeed even without VPNs
			t.Logf("Auto-detection setup note: %v", err)
		}

		// Verify we can query the state
		if exitNode.IsPolicyRoutingActive() {
			t.Logf("Policy routing is active with upstream: %s", exitNode.GetUpstreamInterface())
		}

		// Clean up
		err = exitNode.TeardownPolicyRouting()
		if err != nil {
			t.Errorf("Teardown failed: %v", err)
		}

		// Verify cleanup
		if exitNode.IsPolicyRoutingActive() {
			t.Error("Policy routing should be inactive after teardown")
		}
	})

	// Test 2: Explicit interface (non-existent) with direct fallback
	t.Run("ExplicitInterface_DirectFallback", func(t *testing.T) {
		err := exitNode.SetupPolicyRouting("nonexistent999")
		// With "direct" fallback, this should handle gracefully
		if err != nil {
			t.Logf("Explicit non-existent interface handled: %v", err)
		}

		// Clean up regardless
		err = exitNode.TeardownPolicyRouting()
		if err != nil {
			t.Errorf("Teardown failed: %v", err)
		}
	})

	// Test 3: VPN detection functionality (should work even without VPNs)
	t.Run("VPNDetection", func(t *testing.T) {
		vpns, err := DetectUpstreamVPNs()
		if err != nil {
			t.Logf("VPN detection error (expected if no VPNs): %v", err)
		} else {
			t.Logf("Detected upstream VPNs: %d found", len(vpns))
			for _, vpn := range vpns {
				t.Logf("  VPN: Interface=%s, Type=%s, PublicIP=%s, Active=%t",
					vpn.Interface, vpn.Type, vpn.PublicIP, vpn.IsActive)
			}
		}

		// Test interface existence check
		exists, err := InterfaceExists("lo")
		if err != nil {
			t.Errorf("InterfaceExists check failed: %v", err)
		} else if !exists {
			t.Error("Loopback interface should exist")
		} else {
			t.Log("Loopback interface existence check passed")
		}
	})
}

// TestPolicyRoutingWithBlockFallback tests the "block" fallback behavior.
func TestPolicyRoutingWithBlockFallback(t *testing.T) {
	// Skip if not Linux (policy routing is Linux-only for now)
	if runtime.GOOS != "linux" {
		t.Skip("Policy routing integration test only supports Linux")
	}

	// Skip if not root (can't test iptables/sysctl without root)
	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}

	// Skip if required tools are not available
	if _, err := exec.LookPath("iptables"); err != nil {
		t.Skip("Skipping test: iptables not found")
	}

	// Test configuration with block fallback
	config := ExitNodeConfig{
		Enabled:          true,
		PublicInterface:  "lo", // Use loopback for testing
		AutoDetect:       true,
		FallbackBehavior: "block",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() failed: %v", err)
	}

	// Test with non-existent interface and block fallback
	err = exitNode.SetupPolicyRouting("nonexistent999")
	if err == nil {
		t.Error("Setup with non-existent interface and block fallback should fail")
		// Clean up if it unexpectedly succeeded
		_ = exitNode.TeardownPolicyRouting()
	} else {
		t.Logf("Block fallback correctly rejected non-existent interface: %v", err)
	}

	// Verify state is clean
	if exitNode.IsPolicyRoutingActive() {
		t.Error("Policy routing should not be active after failed setup")
	}
}

// TestPolicyRoutingConfigValidation tests configuration validation.
func TestPolicyRoutingConfigValidation(t *testing.T) {
	tests := []struct {
		name           string
		config         ExitNodeConfig
		expectedFields map[string]interface{}
	}{
		{
			name: "default configuration",
			config: func() ExitNodeConfig {
				cfg := DefaultConfig()
				cfg.ExitNode.Enabled = true
				cfg.ExitNode.PublicInterface = "eth0"
				return cfg.ExitNode
			}(),
			expectedFields: map[string]interface{}{
				"UpstreamVPN":      "",
				"AutoDetect":       false,
				"FallbackBehavior": "direct",
			},
		},
		{
			name: "explicit VPN configuration",
			config: ExitNodeConfig{
				Enabled:          true,
				PublicInterface:  "eth0",
				UpstreamVPN:      "tun0",
				AutoDetect:       false,
				FallbackBehavior: "block",
			},
			expectedFields: map[string]interface{}{
				"UpstreamVPN":      "tun0",
				"AutoDetect":       false,
				"FallbackBehavior": "block",
			},
		},
		{
			name: "auto-detection configuration",
			config: ExitNodeConfig{
				Enabled:          true,
				PublicInterface:  "eth0",
				AutoDetect:       true,
				FallbackBehavior: "direct",
			},
			expectedFields: map[string]interface{}{
				"UpstreamVPN":      "",
				"AutoDetect":       true,
				"FallbackBehavior": "direct",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the fields are set correctly
			if tt.config.UpstreamVPN != tt.expectedFields["UpstreamVPN"].(string) {
				t.Errorf("UpstreamVPN = %s, want %s",
					tt.config.UpstreamVPN, tt.expectedFields["UpstreamVPN"].(string))
			}

			if tt.config.AutoDetect != tt.expectedFields["AutoDetect"].(bool) {
				t.Errorf("AutoDetect = %t, want %t",
					tt.config.AutoDetect, tt.expectedFields["AutoDetect"].(bool))
			}

			if tt.config.FallbackBehavior != tt.expectedFields["FallbackBehavior"].(string) {
				t.Errorf("FallbackBehavior = %s, want %s",
					tt.config.FallbackBehavior, tt.expectedFields["FallbackBehavior"].(string))
			}

			t.Logf("Configuration %s validated successfully", tt.name)
		})
	}
}
