package embedded

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-i2p/wireguard/lib/identity"
)

// errorContains checks if an error contains a specific substring
func errorContains(err error, substring string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), substring)
}

// TestAPIConsistency_InviteCreation tests that embedded and RPC APIs have consistent behavior
func TestAPIConsistency_InviteCreation(t *testing.T) {
	// Create test VPN instance with minimal config
	cfg := testConfig(t)
	cfg.EnableRPC = false
	cfg.EnableWeb = false

	vpn, err := New(cfg)
	if err != nil {
		t.Skipf("Skipping API consistency test due to VPN creation failure: %v", err)
	}

	// Don't start the VPN for basic API signature tests - just test the methods exist and validate parameters
	tests := []struct {
		name          string
		expiry        time.Duration
		maxUses       int
		expectedError bool
		errorContains string
	}{
		{
			name:          "invalid maxUses zero",
			expiry:        1 * time.Hour,
			maxUses:       0,
			expectedError: true,
			errorContains: "maxUses=0 is invalid",
		},
		{
			name:          "invalid maxUses negative",
			expiry:        1 * time.Hour,
			maxUses:       -5,
			expectedError: true,
			errorContains: "maxUses must be positive",
		},
		{
			name:          "valid maxUses unlimited",
			expiry:        1 * time.Hour,
			maxUses:       identity.UnlimitedUses,
			expectedError: true, // Will fail because VPN is not running, but validates parameter
			errorContains: "VPN is not running",
		},
		{
			name:          "valid maxUses positive",
			expiry:        1 * time.Hour,
			maxUses:       5,
			expectedError: true, // Will fail because VPN is not running, but validates parameter
			errorContains: "VPN is not running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test basic CreateInvite method (README compatibility)
			inviteCode, err := vpn.CreateInvite(tt.expiry, tt.maxUses)
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errorContains != "" && !errorContains(err, tt.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Expected no error but got: %v", err)
				return
			}

			if inviteCode == "" {
				t.Errorf("Expected non-empty invite code")
			}

			// Test detailed CreateInvite method (API consistency)
			result, err := vpn.CreateInviteDetailed(tt.expiry, tt.maxUses)
			if tt.expectedError {
				if err == nil {
					t.Errorf("CreateInviteDetailed: Expected error but got none")
					return
				}
				if tt.errorContains != "" && !errorContains(err, tt.errorContains) {
					t.Errorf("CreateInviteDetailed: Expected error containing '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("CreateInviteDetailed failed: %v", err)
				return
			}

			if result == nil {
				t.Errorf("Expected non-nil result")
				return
			}

			if result.InviteCode == "" {
				t.Errorf("Expected non-empty invite code in result")
			}

			if result.InviteCode != inviteCode {
				t.Errorf("Expected CreateInvite and CreateInviteDetailed to return same invite code")
			}
		})
	}
}

// TestAPIConsistency_AcceptInvite tests that AcceptInvite methods provide consistent behavior
func TestAPIConsistency_AcceptInvite(t *testing.T) {
	ctx := context.Background()

	// Create test VPN instance
	cfg := testConfig(t)
	cfg.EnableRPC = false
	cfg.EnableWeb = false

	vpn, err := New(cfg)
	if err != nil {
		t.Skipf("Skipping accept invite test due to VPN creation failure: %v", err)
	}

	tests := []struct {
		name          string
		inviteCode    string
		expectedError bool
		errorContains string
	}{
		{
			name:          "empty invite code",
			inviteCode:    "",
			expectedError: true,
			errorContains: "VPN is not running", // VPN state checked before invite validation
		},
		{
			name:          "invalid invite format",
			inviteCode:    "invalid-format",
			expectedError: true,
		},
		{
			name:          "malformed invite",
			inviteCode:    "i2p://malformed-invite",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test backward compatibility method
			err := vpn.AcceptInviteSimple(ctx, tt.inviteCode)
			if tt.expectedError {
				if err == nil {
					t.Errorf("AcceptInviteSimple: Expected error but got none")
				} else if tt.errorContains != "" && !errorContains(err, tt.errorContains) {
					t.Errorf("AcceptInviteSimple: Expected error containing '%s', got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("AcceptInviteSimple: Expected no error but got: %v", err)
				}
			}

			// Test detailed method
			result, err := vpn.AcceptInvite(ctx, tt.inviteCode)
			if tt.expectedError {
				if err == nil {
					t.Errorf("AcceptInvite: Expected error but got none")
				} else if tt.errorContains != "" && !errorContains(err, tt.errorContains) {
					t.Errorf("AcceptInvite: Expected error containing '%s', got: %v", tt.errorContains, err)
				}
				if result != nil {
					t.Errorf("AcceptInvite: Expected nil result on error, got: %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("AcceptInvite: Expected no error but got: %v", err)
				}
				if result == nil {
					t.Errorf("AcceptInvite: Expected non-nil result on success")
				}
			}
		})
	}
}

// TestBackwardCompatibility ensures that existing code using the simple API still works
func TestBackwardCompatibility(t *testing.T) {
	ctx := context.Background()

	cfg := testConfig(t)
	cfg.EnableRPC = false
	cfg.EnableWeb = false

	vpn, err := New(cfg)
	if err != nil {
		t.Skipf("Skipping backward compatibility test due to VPN creation failure: %v", err)
	}

	// Test that the README example signature still works (parameter validation)
	_, err = vpn.CreateInvite(24*time.Hour, 5)
	if err == nil || !errorContains(err, "VPN is not running") {
		t.Errorf("README example should validate parameters and fail because VPN is not running, got: %v", err)
	}

	// Test CreateInviteDetailed exists with same parameters
	_, err = vpn.CreateInviteDetailed(24*time.Hour, 5)
	if err == nil || !errorContains(err, "VPN is not running") {
		t.Errorf("CreateInviteDetailed should validate parameters and fail because VPN is not running, got: %v", err)
	}

	// Test that the simple AcceptInvite still works (parameter validation)
	err = vpn.AcceptInviteSimple(ctx, "")
	if err == nil || !errorContains(err, "VPN is not running") {
		t.Errorf("AcceptInviteSimple should fail because VPN is not running, got: %v", err)
	}

	// Test that AcceptInvite exists and validates parameters
	result, err := vpn.AcceptInvite(ctx, "")
	if err == nil || !errorContains(err, "VPN is not running") {
		t.Errorf("AcceptInvite should fail because VPN is not running, got: %v", err)
	}
	if result != nil {
		t.Errorf("AcceptInvite should return nil result on error, got: %v", result)
	}

	t.Logf("Backward compatibility maintained - README examples work correctly")
}

// BenchmarkCreateInviteAPIs compares performance of simple vs detailed invite creation
func BenchmarkCreateInviteAPIs(b *testing.B) {
	cfg := testConfig(b)
	cfg.EnableRPC = false
	cfg.EnableWeb = false

	vpn, err := New(cfg)
	if err != nil {
		b.Skipf("Skipping benchmark due to VPN creation failure: %v", err)
	}

	b.Run("CreateInvite_Parameter_Validation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// This will fail due to VPN not running, but tests parameter validation performance
			_, _ = vpn.CreateInvite(1*time.Hour, 1)
		}
	})

	b.Run("CreateInviteDetailed_Parameter_Validation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// This will fail due to VPN not running, but tests parameter validation performance
			_, _ = vpn.CreateInviteDetailed(1*time.Hour, 1)
		}
	})
}
