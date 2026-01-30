package validation

import (
	"strings"
	"testing"
	"time"
)

func TestValidatePeersConnectParams(t *testing.T) {
	tests := []struct {
		name       string
		inviteCode string
		wantErr    bool
	}{
		{"valid", "i2plan://somedata", false},
		{"empty", "", true},
		{"wrong scheme", "http://test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePeersConnectParams(tt.inviteCode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePeersConnectParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInviteCreateParams(t *testing.T) {
	tests := []struct {
		name    string
		expiry  string
		maxUses int
		wantErr bool
		wantDur time.Duration
	}{
		{"defaults", "", 0, false, 0},
		{"valid expiry", "24h", 1, false, 24 * time.Hour},
		{"unlimited uses", "1h", -1, false, time.Hour},
		{"invalid expiry", "invalid", 1, true, 0},
		{"invalid max_uses", "1h", -2, true, 0},
		{"expiry too short", "500ms", 1, true, 0},
		{"expiry too long", "400d", 1, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := ValidateInviteCreateParams(tt.expiry, tt.maxUses)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInviteCreateParams() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && d != tt.wantDur {
				t.Errorf("ValidateInviteCreateParams() duration = %v, want %v", d, tt.wantDur)
			}
		})
	}
}

func TestValidateInviteAcceptParams(t *testing.T) {
	tests := []struct {
		name       string
		inviteCode string
		wantErr    bool
	}{
		{"valid", "i2plan://data", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInviteAcceptParams(tt.inviteCode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInviteAcceptParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfigGetParams(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"empty is valid", "", false},
		{"valid key", "mesh.max_peers", false},
		{"invalid key", "123invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigGetParams(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigGetParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfigSetParams(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   any
		wantErr bool
	}{
		{"valid", "mesh.max_peers", 100, false},
		{"empty key", "", 100, true},
		{"invalid key format", "123bad", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigSetParams(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigSetParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBanAddParams(t *testing.T) {
	validNodeID := strings.Repeat("a", 64)

	tests := []struct {
		name        string
		nodeID      string
		reason      string
		description string
		duration    string
		wantErr     bool
	}{
		{"valid minimal", validNodeID, "", "", "", false},
		{"valid full", validNodeID, "spam", "Spamming the network", "24h", false},
		{"empty node_id", "", "", "", "", true},
		{"invalid node_id", "short", "", "", "", true},
		{"reason too long", validNodeID, strings.Repeat("a", MaxReasonLength+1), "", "", true},
		{"description too long", validNodeID, "", strings.Repeat("a", MaxDescriptionLength+1), "", true},
		{"invalid duration", validNodeID, "", "", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateBanAddParams(tt.nodeID, tt.reason, tt.description, tt.duration)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBanAddParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBanRemoveParams(t *testing.T) {
	validNodeID := strings.Repeat("a", 64)

	tests := []struct {
		name    string
		nodeID  string
		wantErr bool
	}{
		{"valid", validNodeID, false},
		{"empty", "", true},
		{"invalid format", "not-a-node-id", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBanRemoveParams(tt.nodeID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBanRemoveParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
