package mesh

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(StateManagerConfig{
		Path: statePath,
	})

	// Manually set some state
	sm.state.Peers = []PersistedPeer{
		{
			NodeID:      "node-123",
			I2PDest:     "test.b32.i2p",
			WGPublicKey: "ABC123",
			TunnelIP:    "10.42.1.1",
			LastSeen:    time.Now(),
		},
	}
	sm.state.Routes = []PersistedRoute{
		{
			Destination: "10.42.2.0/24",
			NextHop:     "node-456",
			Metric:      2,
			LastUpdated: time.Now(),
		},
	}

	// Save
	if err := sm.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file should exist")
	}

	// Load into new manager
	sm2 := NewStateManager(StateManagerConfig{
		Path: statePath,
	})

	if err := sm2.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify state
	if len(sm2.state.Peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(sm2.state.Peers))
	}
	if sm2.state.Peers[0].NodeID != "node-123" {
		t.Errorf("peer NodeID = %v, want node-123", sm2.state.Peers[0].NodeID)
	}

	if len(sm2.state.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(sm2.state.Routes))
	}
	if sm2.state.Routes[0].NextHop != "node-456" {
		t.Errorf("route NextHop = %v, want node-456", sm2.state.Routes[0].NextHop)
	}
}

func TestStateManager_LoadNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nonexistent.json")

	sm := NewStateManager(StateManagerConfig{
		Path: statePath,
	})

	// Should not error when file doesn't exist
	if err := sm.Load(); err != nil {
		t.Errorf("Load() should not error for nonexistent file: %v", err)
	}
}

func TestStateManager_LoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(statePath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	sm := NewStateManager(StateManagerConfig{
		Path: statePath,
	})

	// Should error on invalid JSON
	if err := sm.Load(); err == nil {
		t.Error("Load() should error for invalid JSON")
	}
}

func TestStateManager_Dirty(t *testing.T) {
	sm := NewStateManager(StateManagerConfig{
		Path: "/tmp/test-state.json",
	})

	if sm.IsDirty() {
		t.Error("new state manager should not be dirty")
	}

	sm.MarkDirty()

	if !sm.IsDirty() {
		t.Error("state manager should be dirty after MarkDirty()")
	}
}

func TestStateManager_Getters(t *testing.T) {
	sm := NewStateManager(StateManagerConfig{
		Path: "/tmp/test-state.json",
	})

	sm.state.Peers = []PersistedPeer{{NodeID: "test"}}
	sm.state.Routes = []PersistedRoute{{Destination: "10.0.0.0/8"}}

	peers := sm.GetPeers()
	if len(peers) != 1 || peers[0].NodeID != "test" {
		t.Error("GetPeers() returned wrong data")
	}

	routes := sm.GetRoutes()
	if len(routes) != 1 || routes[0].Destination != "10.0.0.0/8" {
		t.Error("GetRoutes() returned wrong data")
	}
}

func TestStateManager_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nested", "dir", "state.json")

	sm := NewStateManager(StateManagerConfig{
		Path: statePath,
	})

	if err := sm.Save(); err != nil {
		t.Fatalf("Save() should create nested directory: %v", err)
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("state file should exist in nested directory")
	}
}

func TestStateManager_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(StateManagerConfig{
		Path:         statePath,
		SaveInterval: 10 * time.Millisecond,
	})

	// Start should work
	sm.Start()

	// Let it run for a bit
	time.Sleep(25 * time.Millisecond)

	// Stop should work and do final save
	sm.Stop()

	// Verify file was saved
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("state file should exist after Stop()")
	}
}

func TestStateManager_DoubleStart(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(StateManagerConfig{
		Path:         statePath,
		SaveInterval: 100 * time.Millisecond,
	})

	// First start
	sm.Start()

	// Second start should be a no-op (not panic or create second goroutine)
	sm.Start()

	// Clean up
	sm.Stop()
}

func TestStateManager_DoubleStop(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(StateManagerConfig{
		Path:         statePath,
		SaveInterval: 100 * time.Millisecond,
	})

	sm.Start()

	// First stop
	sm.Stop()

	// Second stop should be a no-op (not panic)
	sm.Stop()
}

func TestStateManager_StopWithoutStart(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(StateManagerConfig{
		Path:         statePath,
		SaveInterval: 100 * time.Millisecond,
	})

	// Stop without Start should be a no-op (not block forever or panic)
	done := make(chan struct{})
	go func() {
		sm.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good - Stop returned
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop() without Start() should return immediately, not block")
	}
}

// TestStateManager_ConcurrentSaveWithDependencies verifies that Save()
// does not cause deadlocks when dependencies are being modified concurrently.
// This test validates the fix for the lock ordering issue documented in AUDIT.md.
func TestStateManager_ConcurrentSaveWithDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create real dependencies
	pm := NewPeerManager(PeerManagerConfig{
		MaxPeers: 100,
	})
	rt := NewRoutingTable(RoutingTableConfig{})

	sm := NewStateManager(StateManagerConfig{
		Path:         statePath,
		SaveInterval: 10 * time.Millisecond,
		PeerManager:  pm,
		RoutingTable: rt,
	})

	// Start auto-save
	sm.Start()
	defer sm.Stop()

	// Run concurrent operations that would deadlock under the old implementation
	// if lock ordering was violated
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			// Simulate peer updates while saving
			pm.mu.Lock()
			// Briefly hold the lock
			pm.mu.Unlock()
		}
	}()

	// Also do manual saves while auto-save is running
	for i := 0; i < 10; i++ {
		if err := sm.Save(); err != nil {
			t.Errorf("Save() error = %v", err)
		}
	}

	// Wait for concurrent operations to complete
	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected - concurrent operations did not complete")
	}
}
