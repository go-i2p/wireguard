package identity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewInviteStore(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")
	if store == nil {
		t.Fatal("NewInviteStore returned nil")
	}

	if store.GeneratedCount() != 0 {
		t.Error("generated count should be 0")
	}
	if store.PendingCount() != 0 {
		t.Error("pending count should be 0")
	}
	if store.AcceptedCount() != 0 {
		t.Error("accepted count should be 0")
	}
}

func TestInviteStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "invites.json")

	// Create identity for invites
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// Create store and add invites
	store := NewInviteStore(storePath)

	inv, err := NewInvite(id, DefaultInviteOptions())
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	key := store.AddGenerated(inv)
	if key == "" {
		t.Error("AddGenerated should return a key")
	}

	if store.GeneratedCount() != 1 {
		t.Errorf("generated count should be 1, got %d", store.GeneratedCount())
	}

	// Save
	if err := store.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := LoadInviteStore(storePath)
	if err != nil {
		t.Fatalf("LoadInviteStore failed: %v", err)
	}

	if loaded.GeneratedCount() != 1 {
		t.Errorf("loaded generated count should be 1, got %d", loaded.GeneratedCount())
	}

	loadedInv, ok := loaded.GetGenerated(key)
	if !ok {
		t.Fatal("GetGenerated should find invite")
	}
	if loadedInv.I2PDest != inv.I2PDest {
		t.Errorf("I2P dest mismatch: got %q, want %q", loadedInv.I2PDest, inv.I2PDest)
	}
}

func TestLoadInviteStore_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "nonexistent.json")

	store, err := LoadInviteStore(storePath)
	if err != nil {
		t.Fatalf("LoadInviteStore should not error on missing file: %v", err)
	}
	if store == nil {
		t.Fatal("store should not be nil")
	}
	if store.GeneratedCount() != 0 {
		t.Error("empty store should have 0 generated")
	}
}

func TestLoadInviteStore_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(storePath, []byte("not valid json"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadInviteStore(storePath)
	if err == nil {
		t.Error("LoadInviteStore should error on invalid JSON")
	}
}

func TestInviteStore_PendingWorkflow(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("network-123")

	inv, err := NewInvite(id, DefaultInviteOptions())
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// Add as pending
	store.AddPending(inv)
	if store.PendingCount() != 1 {
		t.Errorf("pending count should be 1, got %d", store.PendingCount())
	}

	// Get pending
	pending, ok := store.GetPending("network-123")
	if !ok {
		t.Fatal("GetPending should find invite")
	}
	if pending.NetworkID != "network-123" {
		t.Errorf("network ID mismatch: got %q", pending.NetworkID)
	}

	// List pending
	pendingList := store.ListPending()
	if len(pendingList) != 1 {
		t.Errorf("ListPending should return 1, got %d", len(pendingList))
	}

	// Mark as accepted
	if !store.MarkAccepted("network-123") {
		t.Error("MarkAccepted should return true")
	}

	if store.PendingCount() != 0 {
		t.Errorf("pending count should be 0 after accept, got %d", store.PendingCount())
	}
	if store.AcceptedCount() != 1 {
		t.Errorf("accepted count should be 1, got %d", store.AcceptedCount())
	}

	// Get accepted
	accepted, ok := store.GetAccepted("network-123")
	if !ok {
		t.Fatal("GetAccepted should find invite")
	}
	if accepted.NetworkID != "network-123" {
		t.Errorf("network ID mismatch: got %q", accepted.NetworkID)
	}
}

func TestInviteStore_MarkAccepted_NotFound(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	if store.MarkAccepted("nonexistent") {
		t.Error("MarkAccepted should return false for nonexistent invite")
	}
}

func TestInviteStore_RemoveGenerated(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test")

	inv, _ := NewInvite(id, DefaultInviteOptions())
	key := store.AddGenerated(inv)

	if store.GeneratedCount() != 1 {
		t.Errorf("generated count should be 1, got %d", store.GeneratedCount())
	}

	store.RemoveGenerated(key)

	if store.GeneratedCount() != 0 {
		t.Errorf("generated count should be 0 after remove, got %d", store.GeneratedCount())
	}

	_, ok := store.GetGenerated(key)
	if ok {
		t.Error("GetGenerated should not find removed invite")
	}
}

func TestInviteStore_RemovePending(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("network-abc")

	inv, _ := NewInvite(id, DefaultInviteOptions())
	store.AddPending(inv)

	if store.PendingCount() != 1 {
		t.Errorf("pending count should be 1, got %d", store.PendingCount())
	}

	store.RemovePending("network-abc")

	if store.PendingCount() != 0 {
		t.Errorf("pending count should be 0 after remove, got %d", store.PendingCount())
	}
}

func TestInviteStore_CleanExpired(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test")

	// Create an invite with very short expiry
	opts := InviteOptions{
		Expiry:  1 * time.Millisecond,
		MaxUses: 1,
	}
	inv, _ := NewInvite(id, opts)
	store.AddGenerated(inv)

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	if store.GeneratedCount() != 1 {
		t.Errorf("generated count should be 1 before clean, got %d", store.GeneratedCount())
	}

	removed := store.CleanExpired()
	if removed != 1 {
		t.Errorf("CleanExpired should remove 1 invite, removed %d", removed)
	}

	if store.GeneratedCount() != 0 {
		t.Errorf("generated count should be 0 after clean, got %d", store.GeneratedCount())
	}
}

func TestInviteStore_ListGenerated(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test")

	inv1, _ := NewInvite(id, DefaultInviteOptions())
	inv2, _ := NewInvite(id, DefaultInviteOptions())

	store.AddGenerated(inv1)
	store.AddGenerated(inv2)

	list := store.ListGenerated()
	if len(list) != 2 {
		t.Errorf("ListGenerated should return 2, got %d", len(list))
	}
}

func TestInviteStore_AddAccepted(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("direct-accept")

	inv, _ := NewInvite(id, DefaultInviteOptions())

	store.AddAccepted(inv)

	if store.AcceptedCount() != 1 {
		t.Errorf("accepted count should be 1, got %d", store.AcceptedCount())
	}

	accepted, ok := store.GetAccepted("direct-accept")
	if !ok {
		t.Error("GetAccepted should find directly added invite")
	}
	if accepted.NetworkID != "direct-accept" {
		t.Errorf("network ID mismatch: got %q", accepted.NetworkID)
	}
}

func TestInviteStore_ListAccepted(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("network1")

	inv1, _ := NewInvite(id, DefaultInviteOptions())
	store.AddAccepted(inv1)

	id.SetNetworkID("network2")
	inv2, _ := NewInvite(id, DefaultInviteOptions())
	store.AddAccepted(inv2)

	list := store.ListAccepted()
	if len(list) != 2 {
		t.Errorf("ListAccepted should return 2, got %d", len(list))
	}
}
