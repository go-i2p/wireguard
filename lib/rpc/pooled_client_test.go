package rpc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// TestNewPooledClient tests creating a pooled client
func TestNewPooledClient(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server
	srv, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Register a simple status handler
	srv.RegisterHandler("status", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &StatusResult{
			NodeName:  "test-node",
			NodeID:    "test-id",
			State:     "running",
			PeerCount: 0,
			Version:   "1.0.0",
		}, nil
	})

	ctx := context.Background()
	go func() {
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create pooled client
	config := DefaultPooledClientConfig()
	config.UnixSocketPath = socketPath
	config.AuthFile = authFile
	config.PoolSize = 2
	config.MaxIdleTime = 30 * time.Second

	client, err := NewPooledClient(config)
	if err != nil {
		t.Fatalf("NewPooledClient() error = %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Expected non-nil pooled client")
	}
}

// TestPooledClient_Call tests making RPC calls with a pooled client
func TestPooledClient_Call(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server
	srv, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Register a simple status handler
	srv.RegisterHandler("status", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &StatusResult{
			NodeName:  "test-node",
			NodeID:    "test-id",
			State:     "running",
			PeerCount: 0,
			Version:   "1.0.0",
		}, nil
	})

	ctx := context.Background()
	go func() {
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create pooled client
	config := DefaultPooledClientConfig()
	config.UnixSocketPath = socketPath
	config.AuthFile = authFile
	config.PoolSize = 2

	client, err := NewPooledClient(config)
	if err != nil {
		t.Fatalf("NewPooledClient() error = %v", err)
	}
	defer client.Close()

	// Make a call
	result, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if result.NodeID != "test-id" {
		t.Errorf("Status() NodeID = %v, want test-id", result.NodeID)
	}
}

// TestPooledClient_Stats tests getting pool statistics
func TestPooledClient_Stats(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server
	srv, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Register handlers
	srv.RegisterHandler("status", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &StatusResult{NodeID: "test"}, nil
	})

	ctx := context.Background()
	go func() {
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create pooled client
	config := DefaultPooledClientConfig()
	config.UnixSocketPath = socketPath
	config.AuthFile = authFile
	config.PoolSize = 2

	client, err := NewPooledClient(config)
	if err != nil {
		t.Fatalf("NewPooledClient() error = %v", err)
	}
	defer client.Close()

	// Make some calls
	for i := 0; i < 5; i++ {
		_, _ = client.Status(ctx)
	}

	// Get stats
	stats := client.Stats()
	if stats.AcquireCount < 5 {
		t.Errorf("Stats() AcquireCount = %v, want >= 5", stats.AcquireCount)
	}
	if stats.NumOpen == 0 {
		t.Error("Stats() NumOpen = 0, expected connections to be open")
	}
}

// TestPooledClient_MultipleHandlers tests various RPC methods
func TestPooledClient_MultipleHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server
	srv, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Register multiple handlers
	srv.RegisterHandler("status", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &StatusResult{NodeID: "test-id", State: "running"}, nil
	})

	srv.RegisterHandler("peers.list", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &PeersListResult{
			Peers: []PeerInfo{
				{NodeID: "peer1", TunnelIP: "10.42.0.2", State: "connected"},
			},
			Total: 1,
		}, nil
	})

	srv.RegisterHandler("routes.list", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &RoutesListResult{
			Routes: []RouteInfo{
				{TunnelIP: "10.42.0.2", NodeID: "peer1", HopCount: 1, LastSeen: time.Now().Format(time.RFC3339)},
			},
			Total: 1,
		}, nil
	})

	srv.RegisterHandler("bans.list", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &BanListResult{
			Bans: []BanInfo{
				{NodeID: "banned-node", Reason: "misbehavior"},
			},
		}, nil
	})

	srv.RegisterHandler("invite.create", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &InviteCreateResult{
			InviteCode: "i2plan://test-invite",
			ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			MaxUses:    1,
		}, nil
	})

	srv.RegisterHandler("invite.accept", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &InviteAcceptResult{
			NetworkID:  "test-network",
			PeerNodeID: "peer1",
			TunnelIP:   "10.42.1.2",
			Message:    "accepted",
		}, nil
	})

	ctx := context.Background()
	go func() {
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create pooled client
	config := DefaultPooledClientConfig()
	config.UnixSocketPath = socketPath
	config.AuthFile = authFile // Use auth file instead of token
	config.PoolSize = 3

	client, err := NewPooledClient(config)
	if err != nil {
		t.Fatalf("NewPooledClient() error = %v", err)
	}
	defer client.Close()

	// Test Status
	status, err := client.Status(ctx)
	if err != nil {
		t.Errorf("Status() error = %v", err)
	}
	if status.NodeID != "test-id" {
		t.Errorf("Status() NodeID = %v, want test-id", status.NodeID)
	}

	// Test PeersList
	peers, err := client.PeersList(ctx)
	if err != nil {
		t.Errorf("PeersList() error = %v", err)
	}
	if len(peers.Peers) != 1 {
		t.Errorf("PeersList() returned %d peers, want 1", len(peers.Peers))
	}

	// Test RoutesList
	routes, err := client.RoutesList(ctx)
	if err != nil {
		t.Errorf("RoutesList() error = %v", err)
	}
	if len(routes.Routes) != 1 {
		t.Errorf("RoutesList() returned %d routes, want 1", len(routes.Routes))
	}

	// Test BansList
	bans, err := client.BansList(ctx)
	if err != nil {
		t.Errorf("BansList() error = %v", err)
	}
	if len(bans.Bans) != 1 {
		t.Errorf("BansList() returned %d bans, want 1", len(bans.Bans))
	}

	// Test InviteCreate
	invite, err := client.InviteCreate(ctx, "24h", 1)
	if err != nil {
		t.Errorf("InviteCreate() error = %v", err)
	}
	if invite.InviteCode == "" {
		t.Error("InviteCreate() returned empty invite code")
	}

	// Test InviteAccept
	accept, err := client.InviteAccept(ctx, "i2plan://test")
	if err != nil {
		t.Errorf("InviteAccept() error = %v", err)
	}
	if accept.PeerNodeID != "peer1" {
		t.Errorf("InviteAccept() PeerNodeID = %v, want peer1", accept.PeerNodeID)
	}
}

// TestPooledClient_Close tests closing the pooled client
func TestPooledClient_Close(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server
	srv, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()
	go func() {
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create pooled client
	config := DefaultPooledClientConfig()
	config.UnixSocketPath = socketPath
	config.AuthFile = authFile

	client, err := NewPooledClient(config)
	if err != nil {
		t.Fatalf("NewPooledClient() error = %v", err)
	}

	// Close the client
	err = client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Try closing again (should error since pool is closed)
	err = client.Close()
	if err == nil {
		t.Error("Second Close() expected error, got nil")
	}
}
