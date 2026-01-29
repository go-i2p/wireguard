package mesh

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestNewRoutingTable(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	if rt.RouteCount() != 0 {
		t.Errorf("RouteCount() = %d, want 0", rt.RouteCount())
	}
}

func TestRoutingTable_AddGetRoute(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	entry := &RouteEntry{
		TunnelIP:    ip,
		WGPublicKey: key.PublicKey().String(),
		I2PDest:     "test.b32.i2p",
		NodeID:      "node-1",
	}

	err := rt.AddRoute(entry)
	if err != nil {
		t.Fatalf("AddRoute() error = %v", err)
	}

	// Get by IP
	got, ok := rt.GetRoute(ip)
	if !ok {
		t.Fatal("GetRoute() not found")
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %v, want node-1", got.NodeID)
	}

	// Get by NodeID
	got, ok = rt.GetByNodeID("node-1")
	if !ok {
		t.Fatal("GetByNodeID() not found")
	}
	if got.TunnelIP != ip {
		t.Errorf("TunnelIP = %v, want %v", got.TunnelIP, ip)
	}
}

func TestRoutingTable_Collision(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	// Add first entry
	entry1 := &RouteEntry{
		TunnelIP:    ip,
		WGPublicKey: key.PublicKey().String(),
		I2PDest:     "node1.b32.i2p",
		NodeID:      "node-1",
	}
	rt.AddRoute(entry1)

	// Try to add different node with same IP
	entry2 := &RouteEntry{
		TunnelIP:    ip,
		WGPublicKey: key.PublicKey().String(),
		I2PDest:     "node2.b32.i2p",
		NodeID:      "node-2",
	}
	err := rt.AddRoute(entry2)
	if err == nil {
		t.Fatal("AddRoute() should fail with collision")
	}

	if !IsIPCollision(err) {
		t.Errorf("error should be IPCollisionError, got %T", err)
	}

	collisionErr, ok := err.(*IPCollisionError)
	if !ok {
		t.Fatalf("error should be *IPCollisionError")
	}
	if collisionErr.ExistingNode != "node-1" {
		t.Errorf("ExistingNode = %v, want node-1", collisionErr.ExistingNode)
	}
	if collisionErr.NewNode != "node-2" {
		t.Errorf("NewNode = %v, want node-2", collisionErr.NewNode)
	}
}

func TestRoutingTable_HasCollision(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	key1, _ := wgtypes.GeneratePrivateKey()
	ip1 := AllocateTunnelIP(key1.PublicKey())

	// Add first entry
	rt.AddRoute(&RouteEntry{
		TunnelIP:    ip1,
		WGPublicKey: key1.PublicKey().String(),
		NodeID:      "node-1",
	})

	// Check collision with same key, same node - no collision
	if rt.HasCollision(key1.PublicKey(), "node-1") {
		t.Error("HasCollision() should be false for same node")
	}

	// Check collision with same key, different node - collision
	if !rt.HasCollision(key1.PublicKey(), "node-2") {
		t.Error("HasCollision() should be true for different node with same IP")
	}

	// Check collision with different key - no collision (different IP)
	key2, _ := wgtypes.GeneratePrivateKey()
	if rt.HasCollision(key2.PublicKey(), "node-2") {
		t.Error("HasCollision() should be false for different key")
	}
}

func TestRoutingTable_CheckAndAllocateIP(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	key1, _ := wgtypes.GeneratePrivateKey()
	ip1 := AllocateTunnelIP(key1.PublicKey())

	// Allocate for first node
	allocatedIP, err := rt.CheckAndAllocateIP(key1.PublicKey(), "node-1")
	if err != nil {
		t.Fatalf("CheckAndAllocateIP() error = %v", err)
	}
	if allocatedIP != ip1 {
		t.Errorf("allocatedIP = %v, want %v", allocatedIP, ip1)
	}

	// Add the route
	rt.AddRoute(&RouteEntry{
		TunnelIP: ip1,
		NodeID:   "node-1",
	})

	// Try to allocate same IP for different node
	_, err = rt.CheckAndAllocateIP(key1.PublicKey(), "node-2")
	if err == nil {
		t.Error("CheckAndAllocateIP() should fail with collision")
	}
}

func TestRoutingTable_RemoveRoute(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	rt.AddRoute(&RouteEntry{
		TunnelIP: ip,
		NodeID:   "node-1",
	})

	if rt.RouteCount() != 1 {
		t.Fatalf("RouteCount() = %d, want 1", rt.RouteCount())
	}

	// Remove by IP
	removed := rt.RemoveRoute(ip)
	if !removed {
		t.Error("RemoveRoute() returned false")
	}
	if rt.RouteCount() != 0 {
		t.Errorf("RouteCount() after remove = %d, want 0", rt.RouteCount())
	}

	// Test RemoveByNodeID
	rt.AddRoute(&RouteEntry{
		TunnelIP: ip,
		NodeID:   "node-2",
	})
	removed = rt.RemoveByNodeID("node-2")
	if !removed {
		t.Error("RemoveByNodeID() returned false")
	}
	if rt.RouteCount() != 0 {
		t.Errorf("RouteCount() after RemoveByNodeID = %d, want 0", rt.RouteCount())
	}
}

func TestRoutingTable_ExpireStale(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	rt.AddRoute(&RouteEntry{
		TunnelIP: ip,
		NodeID:   "node-1",
	})

	// Expire with very short TTL
	expired := rt.ExpireStale(1 * time.Nanosecond)
	if expired != 1 {
		t.Errorf("ExpireStale() = %d, want 1", expired)
	}
	if rt.RouteCount() != 0 {
		t.Errorf("RouteCount() after expire = %d, want 0", rt.RouteCount())
	}
}

func TestRoutingTable_Merge(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	// Create entries
	entries := make([]*RouteEntry, 3)
	for i := 0; i < 3; i++ {
		key, _ := wgtypes.GeneratePrivateKey()
		entries[i] = &RouteEntry{
			TunnelIP: AllocateTunnelIP(key.PublicKey()),
			NodeID:   string(rune('a' + i)),
			LastSeen: time.Now(),
		}
	}

	added, updated, collisions := rt.Merge(entries)
	if added != 3 {
		t.Errorf("Merge() added = %d, want 3", added)
	}
	if updated != 0 {
		t.Errorf("Merge() updated = %d, want 0", updated)
	}
	if len(collisions) != 0 {
		t.Errorf("Merge() collisions = %d, want 0", len(collisions))
	}

	// Merge again with updated time
	for _, e := range entries {
		e.LastSeen = time.Now().Add(time.Hour)
	}
	added, updated, collisions = rt.Merge(entries)
	if added != 0 {
		t.Errorf("Merge() second added = %d, want 0", added)
	}
	if updated != 3 {
		t.Errorf("Merge() second updated = %d, want 3", updated)
	}
}

func TestRoutingTable_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "routes.json")

	rt := NewRoutingTable(RoutingTableConfig{
		Subnet:   TunnelSubnet,
		FilePath: filePath,
	})

	// Add some routes
	for i := 0; i < 3; i++ {
		key, _ := wgtypes.GeneratePrivateKey()
		rt.AddRoute(&RouteEntry{
			TunnelIP:    AllocateTunnelIP(key.PublicKey()),
			WGPublicKey: key.PublicKey().String(),
			NodeID:      string(rune('a' + i)),
			I2PDest:     "peer.b32.i2p",
		})
	}

	// Save
	err := rt.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("Save() did not create file")
	}

	// Load into new table
	rt2 := NewRoutingTable(RoutingTableConfig{
		Subnet:   TunnelSubnet,
		FilePath: filePath,
	})

	err = rt2.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if rt2.RouteCount() != 3 {
		t.Errorf("RouteCount() after load = %d, want 3", rt2.RouteCount())
	}
}

func TestRoutingTable_UpdateLastSeen(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	key, _ := wgtypes.GeneratePrivateKey()
	ip := AllocateTunnelIP(key.PublicKey())

	rt.AddRoute(&RouteEntry{
		TunnelIP: ip,
		NodeID:   "node-1",
	})

	entry, _ := rt.GetRoute(ip)
	originalTime := entry.LastSeen

	time.Sleep(10 * time.Millisecond)

	updated := rt.UpdateLastSeen(ip)
	if !updated {
		t.Error("UpdateLastSeen() returned false")
	}

	entry, _ = rt.GetRoute(ip)
	if !entry.LastSeen.After(originalTime) {
		t.Error("LastSeen was not updated")
	}
}

func TestRoutingTable_ListRoutes(t *testing.T) {
	rt := NewRoutingTable(RoutingTableConfig{
		Subnet: TunnelSubnet,
	})

	for i := 0; i < 5; i++ {
		key, _ := wgtypes.GeneratePrivateKey()
		rt.AddRoute(&RouteEntry{
			TunnelIP: AllocateTunnelIP(key.PublicKey()),
			NodeID:   string(rune('a' + i)),
		})
	}

	routes := rt.ListRoutes()
	if len(routes) != 5 {
		t.Errorf("ListRoutes() returned %d routes, want 5", len(routes))
	}
}
