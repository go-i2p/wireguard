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

	// Node ID is derived from WG public key, so it should be consistent
	// when derived from the same public key
	derivedID := deriveNodeID(id1.PublicKey())

	if id1.NodeID() != derivedID {
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

	if err := os.WriteFile(idPath, []byte("not valid json"), 0o600); err != nil {
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
	if err := os.WriteFile(idPath, []byte(content), 0o600); err != nil {
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
	if perm != 0o600 {
		t.Errorf("identity file should have 0600 permissions, got %o", perm)
	}
}

func TestIdentity_SignAndVerify(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	message := []byte("test message to sign")

	// Sign the message
	signature := id.Sign(message)

	// Ed25519 signatures are 64 bytes
	if len(signature) != SignatureLength {
		t.Errorf("signature should be %d bytes, got %d", SignatureLength, len(signature))
	}

	// Verify with own key
	if !id.Verify(message, signature) {
		t.Error("signature should verify with own key")
	}

	// Verify with wrong message should fail
	if id.Verify([]byte("wrong message"), signature) {
		t.Error("signature should not verify with wrong message")
	}

	// Verify with wrong signature should fail
	wrongSig := make([]byte, SignatureLength)
	if id.Verify(message, wrongSig) {
		t.Error("wrong signature should not verify")
	}
}

func TestIdentity_VerifyWithPublicKey(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	message := []byte("message for public key verification")
	signature := id.Sign(message)

	// Get the public key
	pubKey := id.VerifyingKey()

	// Verify using the public key directly
	if !VerifyWithPublicKey(pubKey, message, signature) {
		t.Error("should verify with extracted public key")
	}

	// Verify using hex-encoded public key
	pubKeyHex := id.VerifyingKeyHex()
	if !VerifyWithPublicKeyHex(pubKeyHex, message, signature) {
		t.Error("should verify with hex-encoded public key")
	}
}

func TestIdentity_SignMessage(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	message := []byte("message to sign")

	// Create signed message
	signedMsg := id.SignMessage(message)

	// Should be signature + message
	expectedLen := SignatureLength + len(message)
	if len(signedMsg) != expectedLen {
		t.Errorf("signed message should be %d bytes, got %d", expectedLen, len(signedMsg))
	}

	// Extract and verify
	signature, data, ok := ExtractSignedMessage(signedMsg)
	if !ok {
		t.Fatal("failed to extract signed message")
	}

	if string(data) != string(message) {
		t.Error("extracted message should match original")
	}

	if !id.Verify(data, signature) {
		t.Error("extracted signature should verify")
	}
}

func TestExtractSignedMessage_TooShort(t *testing.T) {
	_, _, ok := ExtractSignedMessage(make([]byte, SignatureLength-1))
	if ok {
		t.Error("should fail for message shorter than signature length")
	}
}

func TestVerifyWithPublicKeyHex_InvalidHex(t *testing.T) {
	if VerifyWithPublicKeyHex("not-valid-hex", []byte("msg"), make([]byte, SignatureLength)) {
		t.Error("should fail for invalid hex")
	}
}

func TestVerifyWithPublicKeyHex_WrongLength(t *testing.T) {
	if VerifyWithPublicKeyHex("abcd", []byte("msg"), make([]byte, SignatureLength)) {
		t.Error("should fail for wrong key length")
	}
}

func TestDeriveDiscoveryToken(t *testing.T) {
	// Same network ID should produce same token
	token1 := DeriveDiscoveryToken("test-network-123")
	token2 := DeriveDiscoveryToken("test-network-123")

	if len(token1) != 32 { // SHA-256 produces 32 bytes
		t.Errorf("expected 32 byte token, got %d", len(token1))
	}

	for i := range token1 {
		if token1[i] != token2[i] {
			t.Error("same network ID should produce identical tokens")
			break
		}
	}

	// Different network IDs should produce different tokens
	token3 := DeriveDiscoveryToken("different-network")
	same := true
	for i := range token1 {
		if token1[i] != token3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different network IDs should produce different tokens")
	}
}
