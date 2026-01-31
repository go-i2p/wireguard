package embedded

import (
	"testing"

	"github.com/go-i2p/wireguard/lib/identity"
)

// TestCreateInvite_InvalidMaxUses tests that maxUses=0 is rejected with a clear error message.
// This test addresses the AUDIT.md finding: "Fix MaxUses=0 Semantic Inconsistency"
func TestCreateInvite_InvalidMaxUses(t *testing.T) {
	tests := []struct {
		name      string
		maxUses   int
		wantError bool
		errorMsg  string
	}{
		{
			name:      "zero maxUses should be rejected",
			maxUses:   0,
			wantError: true,
			errorMsg:  "maxUses=0 is invalid",
		},
		{
			name:      "less than -1 should be rejected",
			maxUses:   -2,
			wantError: true,
			errorMsg:  "maxUses must be positive",
		},
		{
			name:      "positive maxUses should work",
			maxUses:   5,
			wantError: false,
		},
		{
			name:      "UnlimitedUses (-1) should work",
			maxUses:   identity.UnlimitedUses,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This test verifies the parameter validation logic
			// without requiring a running VPN. The actual VPN.CreateInvite
			// would require a full node setup which is tested in integration tests.

			// Create a VPN instance (won't be started, just for API validation)
			cfg := Config{
				NodeName: "test-node",
				DataDir:  t.TempDir(),
			}
			vpn, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer vpn.Close()

			// Try to create invite (will fail because VPN not started, but parameter validation happens first)
			_, err = vpn.CreateInvite(24*3600*1000000000, tt.maxUses) // 24 hours in nanoseconds

			if tt.wantError {
				if err == nil {
					t.Errorf("CreateInvite() expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("CreateInvite() error = %q, want error containing %q", err.Error(), tt.errorMsg)
				}
			} else if !tt.wantError && err != nil {
				// For valid parameters, we expect "VPN is not running" error since we didn't start it
				if !contains(err.Error(), "VPN is not running") {
					t.Errorf("CreateInvite() with valid params got unexpected error: %v", err)
				}
			}
		})
	}
}

// contains checks if s contains substr (case-sensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
