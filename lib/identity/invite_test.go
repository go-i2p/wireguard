package identity

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewInvite(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	// Set required fields
	id.SetI2PDest("test-dest.b32.i2p")
	id.SetNetworkID("test-network-123")

	inv, err := NewInvite(id, DefaultInviteOptions())
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	if inv.I2PDest != id.I2PDest() {
		t.Errorf("I2P dest mismatch: got %q, want %q", inv.I2PDest, id.I2PDest())
	}
	if inv.NetworkID != id.NetworkID() {
		t.Errorf("network ID mismatch: got %q, want %q", inv.NetworkID, id.NetworkID())
	}
	if len(inv.AuthToken) != AuthTokenLength {
		t.Errorf("auth token should be %d bytes, got %d", AuthTokenLength, len(inv.AuthToken))
	}
	if inv.MaxUses != DefaultMaxUses {
		t.Errorf("max uses should be %d, got %d", DefaultMaxUses, inv.MaxUses)
	}
	if inv.UsedCount != 0 {
		t.Errorf("used count should be 0, got %d", inv.UsedCount)
	}
	if inv.CreatedBy != id.NodeID() {
		t.Errorf("created by mismatch: got %q, want %q", inv.CreatedBy, id.NodeID())
	}
}

func TestNewInvite_RequiresI2PDest(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetNetworkID("test-network")
	// I2PDest not set

	_, err = NewInvite(id, DefaultInviteOptions())
	if err == nil {
		t.Error("NewInvite should error when I2P dest is not set")
	}
}

func TestNewInvite_RequiresNetworkID(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	// NetworkID not set

	_, err = NewInvite(id, DefaultInviteOptions())
	if err == nil {
		t.Error("NewInvite should error when network ID is not set")
	}
}

func TestInvite_EncodeAndParse(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test-dest.b32.i2p")
	id.SetNetworkID("test-network-123")

	original, err := NewInvite(id, DefaultInviteOptions())
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// Encode
	code, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Should start with scheme
	if !strings.HasPrefix(code, InviteScheme) {
		t.Errorf("invite code should start with %q, got %q", InviteScheme, code)
	}

	// Parse
	parsed, err := ParseInvite(code)
	if err != nil {
		t.Fatalf("ParseInvite failed: %v", err)
	}

	if parsed.I2PDest != original.I2PDest {
		t.Errorf("I2P dest mismatch: got %q, want %q", parsed.I2PDest, original.I2PDest)
	}
	if parsed.NetworkID != original.NetworkID {
		t.Errorf("network ID mismatch: got %q, want %q", parsed.NetworkID, original.NetworkID)
	}
	if !parsed.VerifyToken(original.AuthToken) {
		t.Error("auth token should match")
	}
}

func TestParseInvite_InvalidFormat(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"empty", ""},
		{"no scheme", "abcdef123"},
		{"wrong scheme", "http://abcdef"},
		{"scheme only", InviteScheme},
		{"invalid base64", InviteScheme + "not-valid-base64!!!"},
		{"invalid json", InviteScheme + "bm90LWpzb24="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseInvite(tt.code)
			if err == nil {
				t.Error("ParseInvite should error on invalid format")
			}
		})
	}
}

func TestInvite_Validate_Expired(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// Create invite with very short expiry
	opts := InviteOptions{
		Expiry:  1 * time.Millisecond,
		MaxUses: 1,
	}

	inv, err := NewInvite(id, opts)
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	err = inv.Validate()
	if err != ErrInviteExpired {
		t.Errorf("expected ErrInviteExpired, got %v", err)
	}

	if !inv.IsExpired() {
		t.Error("IsExpired should return true")
	}
}

func TestInvite_Validate_Exhausted(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	opts := InviteOptions{
		Expiry:  24 * time.Hour,
		MaxUses: 1,
	}

	inv, err := NewInvite(id, opts)
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// First use should succeed
	if err := inv.Use(); err != nil {
		t.Errorf("first Use should succeed: %v", err)
	}

	// Second use should fail
	err = inv.Use()
	if err != ErrInviteExhausted {
		t.Errorf("expected ErrInviteExhausted, got %v", err)
	}
}

func TestInvite_RemainingUses(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	opts := InviteOptions{
		Expiry:  24 * time.Hour,
		MaxUses: 3,
	}

	inv, err := NewInvite(id, opts)
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	if inv.RemainingUses() != 3 {
		t.Errorf("remaining uses should be 3, got %d", inv.RemainingUses())
	}

	inv.Use()
	if inv.RemainingUses() != 2 {
		t.Errorf("remaining uses should be 2, got %d", inv.RemainingUses())
	}

	inv.Use()
	inv.Use()
	if inv.RemainingUses() != 0 {
		t.Errorf("remaining uses should be 0, got %d", inv.RemainingUses())
	}
}

func TestInvite_VerifyToken(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	inv, err := NewInvite(id, DefaultInviteOptions())
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// Correct token should verify
	if !inv.VerifyToken(inv.AuthToken) {
		t.Error("correct token should verify")
	}

	// Wrong token should not verify
	wrongToken := make([]byte, AuthTokenLength)
	if inv.VerifyToken(wrongToken) {
		t.Error("wrong token should not verify")
	}

	// Wrong length should not verify
	shortToken := make([]byte, AuthTokenLength-1)
	if inv.VerifyToken(shortToken) {
		t.Error("short token should not verify")
	}
}

func TestInvite_CustomOptions(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	opts := InviteOptions{
		Expiry:  7 * 24 * time.Hour, // 1 week
		MaxUses: 10,
	}

	inv, err := NewInvite(id, opts)
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	if inv.MaxUses != 10 {
		t.Errorf("max uses should be 10, got %d", inv.MaxUses)
	}

	// Expiry should be ~1 week from now
	expectedExpiry := time.Now().Add(7 * 24 * time.Hour)
	diff := inv.ExpiresAt.Sub(expectedExpiry)
	if diff > time.Second || diff < -time.Second {
		t.Errorf("expiry time off by more than 1 second")
	}
}

func TestNewInvite_UnlimitedUses(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// UnlimitedUses (-1) should mean unlimited uses
	opts := InviteOptions{
		Expiry:  24 * time.Hour,
		MaxUses: UnlimitedUses,
	}

	inv, err := NewInvite(id, opts)
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// MaxUses should be -1 (UnlimitedUses)
	if inv.MaxUses != UnlimitedUses {
		t.Errorf("MaxUses should be %d (unlimited), got %d", UnlimitedUses, inv.MaxUses)
	}

	// RemainingUses should return -1 for unlimited
	if inv.RemainingUses() != -1 {
		t.Errorf("RemainingUses should be -1 (unlimited), got %d", inv.RemainingUses())
	}

	// Should be able to use many times without exhaustion
	for i := 0; i < 100; i++ {
		if err := inv.Use(); err != nil {
			t.Fatalf("Use() failed on iteration %d: %v", i, err)
		}
	}

	// Still not exhausted
	if err := inv.Validate(); err != nil {
		t.Errorf("Validate should pass for unlimited invite, got: %v", err)
	}

	// UsedCount should track usage
	if inv.UsedCount != 100 {
		t.Errorf("UsedCount should be 100, got %d", inv.UsedCount)
	}

	// RemainingUses should still be -1 (unlimited)
	if inv.RemainingUses() != -1 {
		t.Errorf("RemainingUses should still be -1 (unlimited), got %d", inv.RemainingUses())
	}
}

func TestNewInvite_NegativeMaxUsesMeansUnlimited(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// Any negative MaxUses means unlimited (same as UnlimitedUses = -1)
	opts := InviteOptions{
		Expiry:  24 * time.Hour,
		MaxUses: -5,
	}

	inv, err := NewInvite(id, opts)
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// Negative values are kept as-is (treated as unlimited)
	if inv.MaxUses != -5 {
		t.Errorf("MaxUses should be -5 (unlimited), got %d", inv.MaxUses)
	}

	// Should be treated as unlimited
	if inv.RemainingUses() != -1 {
		t.Errorf("RemainingUses should be -1 (unlimited), got %d", inv.RemainingUses())
	}
}

// TestParseInvite_MissingNetworkID tests error handling for missing network ID in payload
func TestParseInvite_MissingNetworkID(t *testing.T) {
	// Create an invite without network_id field
	payload := map[string]interface{}{
		"i2p_dest":   "test.b32.i2p",
		"auth_token": "dGVzdHRva2VuZm9ydGVzdGluZ3B1cnBvc2Vz", // base64
	}

	// Encode manually
	jsonData, _ := json.Marshal(payload)
	encoded := base64.URLEncoding.EncodeToString(jsonData)
	code := InviteScheme + encoded

	_, err := ParseInvite(code)
	if err == nil {
		t.Error("ParseInvite should fail when network_id is missing")
	}
}

// TestParseInvite_MissingAuthToken tests error handling for missing auth token in payload
func TestParseInvite_MissingAuthToken(t *testing.T) {
	// Create an invite without auth_token field
	payload := map[string]interface{}{
		"i2p_dest":   "test.b32.i2p",
		"network_id": "test-network",
	}

	// Encode manually
	jsonData, _ := json.Marshal(payload)
	encoded := base64.URLEncoding.EncodeToString(jsonData)
	code := InviteScheme + encoded

	_, err := ParseInvite(code)
	if err == nil {
		t.Error("ParseInvite should fail when auth_token is missing")
	}
}

// TestParseInvite_MissingI2PDest tests error handling for missing I2P dest in payload
func TestParseInvite_MissingI2PDest(t *testing.T) {
	// Create an invite without i2p_dest field
	payload := map[string]interface{}{
		"network_id": "test-network",
		"auth_token": "dGVzdHRva2VuZm9ydGVzdGluZ3B1cnBvc2Vz", // base64
	}

	// Encode manually
	jsonData, _ := json.Marshal(payload)
	encoded := base64.URLEncoding.EncodeToString(jsonData)
	code := InviteScheme + encoded

	_, err := ParseInvite(code)
	if err == nil {
		t.Error("ParseInvite should fail when i2p_dest is missing")
	}
}
