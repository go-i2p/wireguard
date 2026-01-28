package i2pbind

import (
	"testing"

	"github.com/go-i2p/i2pkeys"
)

// getTestAddress returns a valid I2P address for testing.
// Uses the i2pkeys.FiveHundredAs() which provides a valid test destination.
func getTestAddress() i2pkeys.I2PAddr {
	return i2pkeys.FiveHundredAs()
}

func TestNewI2PEndpoint(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	if ep == nil {
		t.Fatal("NewI2PEndpoint returned nil")
	}

	if ep.dest != addr {
		t.Error("Endpoint destination does not match input address")
	}
}

func TestI2PEndpoint_DstToBytes(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	bytes := ep.DstToBytes()

	// The raw hash should be exactly 32 bytes (256 bits)
	// This verifies the fix: we return raw bytes, not base32-encoded string
	if len(bytes) != 32 {
		t.Errorf("DstToBytes should return 32 bytes (raw hash), got %d bytes", len(bytes))
	}
}

func TestI2PEndpoint_DstToString(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	dst := ep.DstToString()

	// Base32 addresses should not be empty
	if dst == "" {
		t.Error("DstToString returned empty string")
	}
}

func TestI2PEndpoint_ClearSrc(t *testing.T) {
	addr := getTestAddress()

	ep := &I2PEndpoint{
		dest: addr,
		src:  addr, // Set a source
	}

	// Before clearing, src should be set (not empty string)
	if ep.src == "" {
		t.Error("Source I2PAddr should not be empty before ClearSrc")
	}

	ep.ClearSrc()

	// After clearing, src should be empty
	if ep.src != "" {
		t.Error("Source should be empty string after ClearSrc")
	}

	// SrcToString should return empty after clearing
	if ep.SrcToString() != "" {
		t.Error("SrcToString should be empty after ClearSrc")
	}
}

func TestI2PEndpoint_SrcToString(t *testing.T) {
	addr := getTestAddress()

	// Test with no source set
	ep := NewI2PEndpoint(addr)
	if ep.SrcToString() != "" {
		t.Error("SrcToString should return empty string when no source is set")
	}

	// Test that setting a source and clearing it works correctly
	ep.src = addr
	// Verify src is actually set (not empty string)
	if ep.src == "" {
		t.Error("Source I2PAddr should be set")
	}

	// Clear and verify
	ep.ClearSrc()
	if ep.SrcToString() != "" {
		t.Error("SrcToString should return empty after ClearSrc")
	}
}

func TestI2PEndpoint_DstIP(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	ip := ep.DstIP()

	// I2P doesn't use IP addresses, so this should return an invalid address
	if ip.IsValid() {
		t.Error("DstIP should return an invalid netip.Addr for I2P endpoints")
	}
}

func TestI2PEndpoint_SrcIP(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	ip := ep.SrcIP()

	// I2P doesn't use IP addresses, so this should return an invalid address
	if ip.IsValid() {
		t.Error("SrcIP should return an invalid netip.Addr for I2P endpoints")
	}
}

func TestI2PEndpoint_Destination(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	dest := ep.Destination()

	if dest != addr {
		t.Error("Destination() should return the original I2P address")
	}
}

func TestNewI2PBind(t *testing.T) {
	bind := NewI2PBind("test-tunnel")
	if bind == nil {
		t.Fatal("NewI2PBind returned nil")
	}

	if bind.name != "test-tunnel" {
		t.Errorf("Expected name 'test-tunnel', got '%s'", bind.name)
	}

	if bind.samAddr != DefaultSAMAddress {
		t.Errorf("Expected default SAM address '%s', got '%s'", DefaultSAMAddress, bind.samAddr)
	}
}

func TestNewI2PBindWithSAM(t *testing.T) {
	customSAM := "192.168.1.1:7656"
	bind := NewI2PBindWithSAM("test-tunnel", customSAM)
	if bind == nil {
		t.Fatal("NewI2PBindWithSAM returned nil")
	}

	if bind.name != "test-tunnel" {
		t.Errorf("Expected name 'test-tunnel', got '%s'", bind.name)
	}

	if bind.samAddr != customSAM {
		t.Errorf("Expected SAM address '%s', got '%s'", customSAM, bind.samAddr)
	}
}

func TestI2PBind_BatchSize(t *testing.T) {
	bind := NewI2PBind("test-tunnel")
	batchSize := bind.BatchSize()

	// I2P doesn't support batching like UDP GSO
	if batchSize != 1 {
		t.Errorf("Expected BatchSize of 1, got %d", batchSize)
	}
}

func TestI2PBind_SetMark(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// SetMark is a no-op for I2P, should always return nil
	err := bind.SetMark(123)
	if err != nil {
		t.Errorf("SetMark should return nil for I2P, got: %v", err)
	}
}

func TestI2PBind_LocalAddressNotOpen(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Before Open() is called, LocalAddress should return an error
	_, err := bind.LocalAddress()
	if err == nil {
		t.Error("LocalAddress should return error when bind is not open")
	}
}

func TestI2PBind_LocalDestinationNotOpen(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Before Open() is called, LocalDestination should return an error
	_, err := bind.LocalDestination()
	if err == nil {
		t.Error("LocalDestination should return error when bind is not open")
	}
}

func TestI2PBind_CloseIdempotent(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Close on an unopened bind should not error
	err := bind.Close()
	if err != nil {
		t.Errorf("First Close should not error: %v", err)
	}

	// Second close should also not error (idempotent)
	err = bind.Close()
	if err != nil {
		t.Errorf("Second Close should not error: %v", err)
	}
}

// TestDstToBytesFormat verifies the fix for the DstToBytes bug.
// Previously, DstToBytes returned a base32-encoded string as bytes (52 bytes).
// After the fix, it returns the raw 32-byte hash.
func TestDstToBytesFormat(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)

	// Get the hash both ways to compare
	hash := addr.DestHash()
	dstBytes := ep.DstToBytes()

	// Raw hash should be exactly 32 bytes
	expectedLen := 32
	if len(dstBytes) != expectedLen {
		t.Errorf("DstToBytes length mismatch: expected %d bytes, got %d bytes", expectedLen, len(dstBytes))
	}

	// The bytes should match the raw hash array
	for i := 0; i < len(hash); i++ {
		if dstBytes[i] != hash[i] {
			t.Errorf("DstToBytes byte mismatch at index %d: expected %d, got %d", i, hash[i], dstBytes[i])
			break
		}
	}
}
