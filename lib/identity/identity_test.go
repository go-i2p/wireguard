package identity

import (
	"os"
	"path/filepath"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestNewIdentity(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	// Check that keys are set
	if id.PrivateKey() == (wgtypes.Key{}) {
		t.Error("private key should not be zero")
	}
	if id.PublicKey() == (wgtypes.Key{}) {
		t.Error("public key should not be zero")
	}

	// Check node ID
	if id.NodeID() == "" {
		t.Error("node ID should not be empty")
	}
	if len(id.NodeID()) != NodeIDLength*2 { // hex encoding doubles length
		t.Errorf("node ID should be %d hex chars, got %d", NodeIDLength*2, len(id.NodeID()))
	}

	// I2P dest should be empty initially
	if id.I2PDest() != "" {
		t.Error("I2P dest should be empty initially")
	}

	// Network ID should be empty initially
	if id.NetworkID() != "" {
		t.Error("network ID should be empty initially")
	}

	// CreatedAt should be set
	if id.CreatedAt().IsZero() {
		t.Error("created at should be set")
	}
}

func TestIdentity_DeriveNodeID_Deterministic(t *testing.T) {
	id1, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	// Create another identity with same private key
	id2 := newIdentityFromPrivateKey(id1.PrivateKey())

	if id1.NodeID() != id2.NodeID() {
		t.Error("node ID should be deterministic from public key")
	}
}

func TestIdentity_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "identity.json")

	// Create and save
	original, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	original.SetI2PDest("test-dest.b32.i2p")
	original.SetNetworkID("test-network-123")

	if err := original.Save(idPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load and verify
	loaded, err := LoadIdentity(idPath)
	if err != nil {
		t.Fatalf("LoadIdentity failed: %v", err)
	}

	if loaded.PrivateKey() != original.PrivateKey() {
		t.Error("private key mismatch")
	}
	if loaded.PublicKey() != original.PublicKey() {
		t.Error("public key mismatch")
	}
	if loaded.NodeID() != original.NodeID() {
		t.Error("node ID mismatch")
	}
	if loaded.I2PDest() != original.I2PDest() {
		t.Errorf("I2P dest mismatch: got %q, want %q", loaded.I2PDest(), original.I2PDest())
	}
	if loaded.NetworkID() != original.NetworkID() {
		t.Errorf("network ID mismatch: got %q, want %q", loaded.NetworkID(), original.NetworkID())
	}
}

func TestLoadIdentity_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "nonexistent.json")

	id, err := LoadIdentity(idPath)
	if err != nil {
		t.Fatalf("LoadIdentity should not error on missing file: %v", err)
	}
	if id != nil {
		t.Error("LoadIdentity should return nil for missing file")
	}
}

func TestLoadIdentity_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(idPath, []byte("not valid json"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadIdentity(idPath)
	if err == nil {
		t.Error("LoadIdentity should error on invalid JSON")
	}
}

func TestLoadIdentity_InvalidPrivateKey(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "bad-key.json")

	content := `{"private_key":"not-a-valid-key","public_key":"x","node_id":"y","created_at":"2024-01-01T00:00:00Z"}`
	if err := os.WriteFile(idPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadIdentity(idPath)
	if err == nil {
		t.Error("LoadIdentity should error on invalid private key")
	}
}

func TestIdentity_SetI2PDest(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	if id.I2PDest() != "" {
		t.Error("I2P dest should be empty initially")
	}

	id.SetI2PDest("abc123.b32.i2p")
	if id.I2PDest() != "abc123.b32.i2p" {
		t.Errorf("I2P dest not set correctly: got %q", id.I2PDest())
	}
}

func TestIdentity_SetNetworkID(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	if id.NetworkID() != "" {
		t.Error("network ID should be empty initially")
	}

	id.SetNetworkID("network-abc")
	if id.NetworkID() != "network-abc" {
		t.Errorf("network ID not set correctly: got %q", id.NetworkID())
	}
}

func TestGenerateNetworkID(t *testing.T) {
	id1, err := GenerateNetworkID()
	if err != nil {
		t.Fatalf("GenerateNetworkID failed: %v", err)
	}

	id2, err := GenerateNetworkID()
	if err != nil {
		t.Fatalf("GenerateNetworkID failed: %v", err)
	}

	// IDs should be 32 hex chars (16 bytes)
	if len(id1) != 32 {
		t.Errorf("network ID should be 32 hex chars, got %d", len(id1))
	}

	// Should be unique
	if id1 == id2 {
		t.Error("generated network IDs should be unique")
	}
}

func TestIdentity_Save_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "nested", "dir", "identity.json")

	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	if err := id.Save(idPath); err != nil {
		t.Fatalf("Save failed to create nested directory: %v", err)
	}

	if _, err := os.Stat(idPath); os.IsNotExist(err) {
		t.Error("identity file was not created")
	}
}

func TestIdentity_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "identity.json")

	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	if err := id.Save(idPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := os.Stat(idPath)
	if err != nil {
		t.Fatalf("failed to stat identity file: %v", err)
	}

	// File should have restricted permissions (0600)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("identity file should have 0600 permissions, got %o", perm)
	}
}
