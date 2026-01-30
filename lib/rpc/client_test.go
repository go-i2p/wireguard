package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewClient_UnixSocket tests creating a client with Unix socket.
func TestNewClient_UnixSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server
	srv, err := NewServer(ServerConfig{
		UnixSocketPath: socketPath,
		AuthFile:       authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	go func() {
		ctx := context.Background()
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Create client using the same auth file
	client, err := NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		AuthFile:       authFile,
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if client.conn == nil {
		t.Error("client connection is nil")
	}
}

// TestNewClient_TCPAddress tests creating a client with TCP address.
func TestNewClient_TCPAddress(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server on TCP
	srv, err := NewServer(ServerConfig{
		TCPAddress: "127.0.0.1:0", // Random available port
		AuthFile:   authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	go func() {
		ctx := context.Background()
		_ = srv.Start(ctx, ServerConfig{TCPAddress: "127.0.0.1:0"})
	}()
	defer srv.Stop()

	// Wait for server to start and get the actual port
	time.Sleep(50 * time.Millisecond)
	tcpAddr := srv.TCPAddress()
	if tcpAddr == "" {
		t.Skip("Server did not start with TCP address")
	}

	// Create client
	client, err := NewClient(ClientConfig{
		TCPAddress: tcpAddr,
		AuthFile:   authFile,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if client.conn == nil {
		t.Error("client connection is nil")
	}
}

// TestNewClient_AuthFile tests loading auth token from file.
func TestNewClient_AuthFile(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Create a test server first to generate auth token
	srv, err := NewServer(ServerConfig{
		UnixSocketPath: socketPath,
		AuthFile:       authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	go func() {
		ctx := context.Background()
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create client with auth file
	client, err := NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		AuthFile:       authFile,
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if client.conn == nil {
		t.Error("client connection is nil")
	}
}

// TestNewClient_NoAddress tests error when no address is provided.
func TestNewClient_NoAddress(t *testing.T) {
	_, err := NewClient(ClientConfig{
		AuthToken: "test-token",
		Timeout:   5 * time.Second,
	})
	if err == nil {
		t.Error("NewClient() expected error for no address, got nil")
	}
}

// TestNewClient_InvalidAuthFile tests error for non-existent auth file.
func TestNewClient_InvalidAuthFile(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a test server
	srv, err := NewServer(ServerConfig{
		UnixSocketPath: socketPath,
		AuthFile:       filepath.Join(tmpDir, "server_auth.token"),
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	go func() {
		ctx := context.Background()
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Try to create client with non-existent auth file
	_, err = NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		AuthFile:       "/nonexistent/auth.token",
		Timeout:        5 * time.Second,
	})
	if err == nil {
		t.Error("NewClient() expected error for invalid auth file, got nil")
	}
}

// TestNewClient_ConnectionRefused tests error when server is not running.
func TestNewClient_ConnectionRefused(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "nonexistent.sock")

	_, err := NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		AuthToken:      "test-token",
		Timeout:        1 * time.Second,
	})
	if err == nil {
		t.Error("NewClient() expected error for connection refused, got nil")
	}
}

// setupTestClientServer creates a test server with mock handlers and returns a connected client
func setupTestClientServer(t *testing.T) (*Client, *Server, func()) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create auth file
	authFile := filepath.Join(tmpDir, "auth.txt")
	if err := os.WriteFile(authFile, []byte(hex.EncodeToString([]byte("test-auth-token"))), 0600); err != nil {
		t.Fatal(err)
	}

	// Create server
	srv, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Register mock handlers
	srv.RegisterHandler("status", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &StatusResult{
			NodeName:  "test-node",
			NodeID:    "test-node-123",
			State:     "running",
			PeerCount: 5,
			Uptime:    "1h",
			Version:   "1.0.0",
		}, nil
	})

	srv.RegisterHandler("peers.list", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &PeersListResult{
			Peers: []PeerInfo{
				{NodeID: "peer1", TunnelIP: "10.42.0.2", State: "connected"},
				{NodeID: "peer2", TunnelIP: "10.42.0.3", State: "connected"},
			},
			Total: 2,
		}, nil
	})

	srv.RegisterHandler("peers.connect", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &PeersConnectResult{
			NodeID:   "new-peer",
			TunnelIP: "10.42.1.1",
			Message:  "connected",
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
			PeerNodeID: "invited-peer",
			TunnelIP:   "10.42.1.2",
			Message:    "accepted",
		}, nil
	})

	srv.RegisterHandler("routes.list", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &RoutesListResult{
			Routes: []RouteInfo{
				{TunnelIP: "10.42.0.2", NodeID: "node1", HopCount: 1, LastSeen: time.Now().Format(time.RFC3339)},
			},
			Total: 1,
		}, nil
	})

	srv.RegisterHandler("config.get", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &ConfigGetResult{
			Key:   "test-key",
			Value: "test-value",
		}, nil
	})

	srv.RegisterHandler("config.set", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &ConfigSetResult{Message: "config updated"}, nil
	})

	srv.RegisterHandler("bans.list", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &BanListResult{
			Bans: []BanInfo{
				{NodeID: "banned-node", Reason: "test-reason"},
			},
		}, nil
	})

	srv.RegisterHandler("bans.add", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &BanAddResult{Message: "ban added"}, nil
	})

	srv.RegisterHandler("bans.remove", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return &BanRemoveResult{Message: "ban removed"}, nil
	})

	// Start server
	ctx := context.Background()
	if err := srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath}); err != nil {
		t.Fatal(err)
	}

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Create client
	client, err := NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		AuthToken:      srv.AuthToken(),
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	cleanup := func() {
		client.Close()
		srv.Stop()
	}

	return client, srv, cleanup
}

// TestClient_Status tests the Status method
func TestClient_Status(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if result.NodeID != "test-node-123" {
		t.Errorf("Status() NodeID = %v, want test-node-123", result.NodeID)
	}
	if result.PeerCount != 5 {
		t.Errorf("Status() PeerCount = %v, want 5", result.PeerCount)
	}
}

// TestClient_PeersList tests the PeersList method
func TestClient_PeersList(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.PeersList(ctx)
	if err != nil {
		t.Fatalf("PeersList() error = %v", err)
	}

	if len(result.Peers) != 2 {
		t.Errorf("PeersList() returned %d peers, want 2", len(result.Peers))
	}
}

// TestClient_PeersConnect tests the PeersConnect method
func TestClient_PeersConnect(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.PeersConnect(ctx, "i2plan://test-invite")
	if err != nil {
		t.Fatalf("PeersConnect() error = %v", err)
	}

	if result.NodeID != "new-peer" {
		t.Errorf("PeersConnect() NodeID = %v, want new-peer", result.NodeID)
	}
	if result.Message != "connected" {
		t.Errorf("PeersConnect() Message = %v, want 'connected'", result.Message)
	}
}

// TestClient_InviteCreate tests the InviteCreate method
func TestClient_InviteCreate(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.InviteCreate(ctx, "24h", 1)
	if err != nil {
		t.Fatalf("InviteCreate() error = %v", err)
	}

	if result.InviteCode == "" {
		t.Error("InviteCreate() returned empty invite code")
	}
	if result.MaxUses != 1 {
		t.Errorf("InviteCreate() MaxUses = %v, want 1", result.MaxUses)
	}
}

// TestClient_InviteAccept tests the InviteAccept method
func TestClient_InviteAccept(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.InviteAccept(ctx, "i2plan://test-invite")
	if err != nil {
		t.Fatalf("InviteAccept() error = %v", err)
	}

	if result.PeerNodeID != "invited-peer" {
		t.Errorf("InviteAccept() PeerNodeID = %v, want 'invited-peer'", result.PeerNodeID)
	}
	if result.NetworkID != "test-network" {
		t.Errorf("InviteAccept() NetworkID = %v, want 'test-network'", result.NetworkID)
	}
}

// TestClient_RoutesList tests the RoutesList method
func TestClient_RoutesList(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.RoutesList(ctx)
	if err != nil {
		t.Fatalf("RoutesList() error = %v", err)
	}

	if len(result.Routes) != 1 {
		t.Errorf("RoutesList() returned %d routes, want 1", len(result.Routes))
	}
}

// TestClient_ConfigGet tests the ConfigGet method
func TestClient_ConfigGet(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.ConfigGet(ctx, "test-key")
	if err != nil {
		t.Fatalf("ConfigGet() error = %v", err)
	}

	if result.Key != "test-key" {
		t.Errorf("ConfigGet() Key = %v, want test-key", result.Key)
	}
	if result.Value != "test-value" {
		t.Errorf("ConfigGet() Value = %v, want test-value", result.Value)
	}
}

// TestClient_ConfigSet tests the ConfigSet method
func TestClient_ConfigSet(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.ConfigSet(ctx, "test-key", "new-value")
	if err != nil {
		t.Fatalf("ConfigSet() error = %v", err)
	}

	if result.Message != "config updated" {
		t.Errorf("ConfigSet() Message = %v, want 'config updated'", result.Message)
	}
}

// TestClient_BansList tests the BansList method
func TestClient_BansList(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.BansList(ctx)
	if err != nil {
		t.Fatalf("BansList() error = %v", err)
	}

	if len(result.Bans) != 1 {
		t.Errorf("BansList() returned %d bans, want 1", len(result.Bans))
	}
}

// TestClient_BansAdd tests the BansAdd method
func TestClient_BansAdd(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.BansAdd(ctx, "bad-node", "misbehavior", "test description", "1h")
	if err != nil {
		t.Fatalf("BansAdd() error = %v", err)
	}

	if result.Message != "ban added" {
		t.Errorf("BansAdd() Message = %v, want 'ban added'", result.Message)
	}
}

// TestClient_BansRemove tests the BansRemove method
func TestClient_BansRemove(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := client.BansRemove(ctx, "banned-node")
	if err != nil {
		t.Fatalf("BansRemove() error = %v", err)
	}

	if result.Message != "ban removed" {
		t.Errorf("BansRemove() Message = %v, want 'ban removed'", result.Message)
	}
}

// TestClient_ContextCancellation tests that operations respect context cancellation
func TestClient_ContextCancellation(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Status(ctx)
	if err == nil {
		t.Error("Status() with cancelled context expected error, got nil")
	}
}

// TestClient_Close tests closing the client connection
func TestClient_Close(t *testing.T) {
	client, _, cleanup := setupTestClientServer(t)
	defer cleanup()

	err := client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify connection is closed by trying to use it
	ctx := context.Background()
	_, err = client.Status(ctx)
	if err == nil {
		t.Error("Status() after Close() expected error, got nil")
	}
}

// TestClient_DefaultTimeout tests that default timeout is set
func TestClient_DefaultTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a test server
	authorFile := filepath.Join(tmpDir, "auth.txt")
	if err := os.WriteFile(authorFile, []byte(hex.EncodeToString([]byte("test-auth-token"))), 0600); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(ServerConfig{
		UnixSocketPath: socketPath,
		AuthFile:       authorFile,
	})
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		ctx := context.Background()
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create client without timeout (should use default)
	client, err := NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		AuthToken:      hex.EncodeToString([]byte("test-auth-token")),
		// Timeout not set - should default to 30s
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if client.timeout != 30*time.Second {
		t.Errorf("Default timeout = %v, want 30s", client.timeout)
	}
}

// TestClient_InvalidAuthToken tests authentication failure
func TestClient_InvalidAuthToken(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	authFile := filepath.Join(tmpDir, "auth.token")

	// Start a test server with auth
	srv, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	go func() {
		ctx := context.Background()
		_ = srv.Start(ctx, ServerConfig{UnixSocketPath: socketPath})
	}()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// Try to connect with wrong auth token
	client, err := NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		AuthToken:      hex.EncodeToString([]byte("wrong-token")),
		Timeout:        5 * time.Second,
	})
	if err == nil {
		defer client.Close()
		t.Error("NewClient() with wrong auth token expected error, got nil")
	}
}

// TestClient_CallWithNetworkError tests error handling for network failures
func TestClient_CallWithNetworkError(t *testing.T) {
	// Create a connection that will fail
	// Use a pipe to simulate a connection that gets closed
	server, client := net.Pipe()

	c := &Client{
		conn:      client,
		authToken: []byte("test"),
		requestID: 0,
		timeout:   5 * time.Second,
	}

	// Close the server side to cause network error
	server.Close()
	client.Close()

	// Try to make a call - should fail with network error
	ctx := context.Background()
	_, err := c.Status(ctx)
	if err == nil {
		t.Error("Status() with closed connection expected error, got nil")
	}
}
