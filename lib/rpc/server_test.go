package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	t.Run("without auth file", func(t *testing.T) {
		s, err := NewServer(ServerConfig{})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		if s == nil {
			t.Fatal("server is nil")
		}
		if s.AuthToken() != "" {
			t.Error("expected no auth token")
		}
	})

	t.Run("with auth file", func(t *testing.T) {
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "auth.token")

		s, err := NewServer(ServerConfig{
			AuthFile: authFile,
		})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}

		// Token should be generated
		token := s.AuthToken()
		if token == "" {
			t.Fatal("expected auth token")
		}
		if len(token) != AuthTokenLength*2 { // hex encoded
			t.Errorf("expected token length %d, got %d", AuthTokenLength*2, len(token))
		}

		// File should be created
		data, err := os.ReadFile(authFile)
		if err != nil {
			t.Fatalf("reading auth file: %v", err)
		}
		if string(data) != token {
			t.Error("auth file content mismatch")
		}
	})

	t.Run("loads existing auth file", func(t *testing.T) {
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "auth.token")

		// Create token file
		expectedToken := make([]byte, AuthTokenLength)
		for i := range expectedToken {
			expectedToken[i] = byte(i)
		}
		if err := os.WriteFile(authFile, []byte(hex.EncodeToString(expectedToken)), 0o600); err != nil {
			t.Fatalf("writing auth file: %v", err)
		}

		s, err := NewServer(ServerConfig{
			AuthFile: authFile,
		})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}

		if s.AuthToken() != hex.EncodeToString(expectedToken) {
			t.Error("auth token mismatch")
		}
	})
}

func TestServerRegisterHandler(t *testing.T) {
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	handler := func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return map[string]string{"status": "ok"}, nil
	}

	s.RegisterHandler("test.method", handler)

	// Check handler is registered
	s.mu.RLock()
	_, ok := s.handlers["test.method"]
	s.mu.RUnlock()

	if !ok {
		t.Error("handler not registered")
	}
}

func TestServerRegisterHandlers(t *testing.T) {
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	handlers := map[string]Handler{
		"method1": func(ctx context.Context, params json.RawMessage) (any, *Error) {
			return "result1", nil
		},
		"method2": func(ctx context.Context, params json.RawMessage) (any, *Error) {
			return "result2", nil
		},
	}

	s.RegisterHandlers(handlers)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.handlers["method1"]; !ok {
		t.Error("method1 not registered")
	}
	if _, ok := s.handlers["method2"]; !ok {
		t.Error("method2 not registered")
	}
}

func TestServerStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	if err := s.Start(ctx, ServerConfig{UnixSocketPath: socketPath}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !s.IsRunning() {
		t.Error("server should be running")
	}

	if s.UnixSocketPath() == "" {
		t.Error("expected unix socket path")
	}

	// Socket file should exist
	if _, err := os.Stat(socketPath); err != nil {
		t.Errorf("socket file not found: %v", err)
	}

	// Stop server
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if s.IsRunning() {
		t.Error("server should not be running")
	}
}

func TestServerStartTCP(t *testing.T) {
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server on random port
	if err := s.Start(ctx, ServerConfig{TCPAddress: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	if s.TCPAddress() == "" {
		t.Error("expected TCP address")
	}
}

func TestServerStopWithContext(t *testing.T) {
	t.Run("graceful shutdown", func(t *testing.T) {
		tmpDir := t.TempDir()
		socketPath := filepath.Join(tmpDir, "test.sock")

		s, err := NewServer(ServerConfig{})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}

		ctx := context.Background()
		if err := s.Start(ctx, ServerConfig{UnixSocketPath: socketPath}); err != nil {
			t.Fatalf("Start: %v", err)
		}

		// Stop with generous timeout
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.StopWithContext(stopCtx); err != nil {
			t.Errorf("StopWithContext: %v", err)
		}

		if s.IsRunning() {
			t.Error("server should not be running after StopWithContext")
		}
	})

	t.Run("stop already stopped", func(t *testing.T) {
		s, err := NewServer(ServerConfig{})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}

		// Stopping a server that was never started should be a no-op
		if err := s.StopWithContext(context.Background()); err != nil {
			t.Errorf("StopWithContext on stopped server: %v", err)
		}
	})
}

func TestServerStartNoListeners(t *testing.T) {
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx := context.Background()

	// Start without any listeners should fail
	if err := s.Start(ctx, ServerConfig{}); err == nil {
		t.Error("expected error when starting without listeners")
	}
}

func TestServerDoubleStart(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx := context.Background()
	cfg := ServerConfig{UnixSocketPath: socketPath}

	if err := s.Start(ctx, cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	// Second start should fail
	if err := s.Start(ctx, cfg); err == nil {
		t.Error("expected error on double start")
	}
}

func TestServerDispatch(t *testing.T) {
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	s.RegisterHandler("echo", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		var p map[string]string
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ErrInvalidParams(err.Error())
		}
		return p, nil
	})

	s.RegisterHandler("error", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return nil, ErrInternal("test error")
	})

	t.Run("successful dispatch", func(t *testing.T) {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "echo",
			Params:  json.RawMessage(`{"msg":"hello"}`),
			ID:      json.RawMessage(`1`),
		}

		resp := s.dispatch(context.Background(), req)
		if resp.Error != nil {
			t.Errorf("unexpected error: %v", resp.Error)
		}
	})

	t.Run("error dispatch", func(t *testing.T) {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "error",
			ID:      json.RawMessage(`2`),
		}

		resp := s.dispatch(context.Background(), req)
		if resp.Error == nil {
			t.Error("expected error")
		}
		if resp.Error.Code != ErrCodeInternal {
			t.Errorf("expected internal error code, got %d", resp.Error.Code)
		}
	})

	t.Run("method not found", func(t *testing.T) {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "unknown",
			ID:      json.RawMessage(`3`),
		}

		resp := s.dispatch(context.Background(), req)
		if resp.Error == nil {
			t.Error("expected error")
		}
		if resp.Error.Code != ErrCodeMethodNotFound {
			t.Errorf("expected method not found error code, got %d", resp.Error.Code)
		}
	})
}

func TestServerHandleAuth(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth.token")

	s, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	token := s.AuthToken()

	t.Run("valid auth", func(t *testing.T) {
		authenticated := false
		req := &Request{
			JSONRPC: "2.0",
			Method:  "auth",
			Params:  json.RawMessage(`{"token":"` + token + `"}`),
			ID:      json.RawMessage(`1`),
		}

		resp := s.handleAuth(req, &authenticated)
		if resp.Error != nil {
			t.Errorf("unexpected error: %v", resp.Error)
		}
		if !authenticated {
			t.Error("expected authenticated to be true")
		}
	})

	t.Run("invalid auth", func(t *testing.T) {
		authenticated := false
		req := &Request{
			JSONRPC: "2.0",
			Method:  "auth",
			Params:  json.RawMessage(`{"token":"invalid"}`),
			ID:      json.RawMessage(`2`),
		}

		resp := s.handleAuth(req, &authenticated)
		if resp.Error == nil {
			t.Error("expected error for invalid token")
		}
		if authenticated {
			t.Error("expected authenticated to be false")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		authenticated := false
		req := &Request{
			JSONRPC: "2.0",
			Method:  "auth",
			Params:  json.RawMessage(`{}`),
			ID:      json.RawMessage(`3`),
		}

		resp := s.handleAuth(req, &authenticated)
		if resp.Error == nil {
			t.Error("expected error for missing token")
		}
	})
}

func TestServerNoAuthRequired(t *testing.T) {
	// Server without auth file
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	authenticated := false
	req := &Request{
		JSONRPC: "2.0",
		Method:  "auth",
		ID:      json.RawMessage(`1`),
	}

	resp := s.handleAuth(req, &authenticated)
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
	if !authenticated {
		t.Error("expected authenticated to be true when no auth required")
	}
}

func TestLoadOrCreateAuthToken(t *testing.T) {
	t.Run("creates new token", func(t *testing.T) {
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "subdir", "auth.token")

		s, err := NewServer(ServerConfig{})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		token, err := s.loadOrCreateAuthToken(authFile)
		if err != nil {
			t.Fatalf("loadOrCreateAuthToken: %v", err)
		}

		if len(token) != AuthTokenLength {
			t.Errorf("expected token length %d, got %d", AuthTokenLength, len(token))
		}

		// File should exist
		data, err := os.ReadFile(authFile)
		if err != nil {
			t.Fatalf("reading file: %v", err)
		}

		// File should contain hex-encoded token
		decoded, err := hex.DecodeString(string(data))
		if err != nil {
			t.Fatalf("decoding: %v", err)
		}

		if string(decoded) != string(token) {
			t.Error("file content mismatch")
		}
	})

	t.Run("regenerates invalid token", func(t *testing.T) {
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "auth.token")

		// Write invalid token
		if err := os.WriteFile(authFile, []byte("not-valid-hex"), 0o600); err != nil {
			t.Fatalf("writing file: %v", err)
		}

		s, err := NewServer(ServerConfig{})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		token, err := s.loadOrCreateAuthToken(authFile)
		if err != nil {
			t.Fatalf("loadOrCreateAuthToken: %v", err)
		}

		if len(token) != AuthTokenLength {
			t.Errorf("expected new token length %d, got %d", AuthTokenLength, len(token))
		}
	})
}

func TestIntegrationClientServer(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create server
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Register test handler
	s.RegisterHandler("ping", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return map[string]string{"message": "pong"}, nil
	})

	s.RegisterHandler("echo", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		var p map[string]any
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ErrInvalidParams(err.Error())
		}
		return p, nil
	})

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx, ServerConfig{UnixSocketPath: socketPath}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Create client
	c, err := NewClient(ClientConfig{
		UnixSocketPath: socketPath,
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	t.Run("ping", func(t *testing.T) {
		var result map[string]string
		if err := c.Call(context.Background(), "ping", nil, &result); err != nil {
			t.Fatalf("Call: %v", err)
		}
		if result["message"] != "pong" {
			t.Errorf("expected pong, got %s", result["message"])
		}
	})

	t.Run("echo", func(t *testing.T) {
		params := map[string]string{"test": "value"}
		var result map[string]string
		if err := c.Call(context.Background(), "echo", params, &result); err != nil {
			t.Fatalf("Call: %v", err)
		}
		if result["test"] != "value" {
			t.Errorf("expected value, got %s", result["test"])
		}
	})

	t.Run("method not found", func(t *testing.T) {
		var result any
		err := c.Call(context.Background(), "unknown", nil, &result)
		if err == nil {
			t.Fatal("expected error")
		}
		rpcErr, ok := err.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T", err)
		}
		if rpcErr.Code != ErrCodeMethodNotFound {
			t.Errorf("expected code %d, got %d", ErrCodeMethodNotFound, rpcErr.Code)
		}
	})
}

func TestIntegrationWithAuth(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth.token")

	// Create server with auth
	s, err := NewServer(ServerConfig{
		AuthFile: authFile,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	s.RegisterHandler("secure", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return map[string]string{"data": "secret"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start on TCP (which requires auth)
	if err := s.Start(ctx, ServerConfig{TCPAddress: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	time.Sleep(50 * time.Millisecond)

	tcpAddr := s.TCPAddress()

	t.Run("with valid auth", func(t *testing.T) {
		c, err := NewClient(ClientConfig{
			TCPAddress: tcpAddr,
			AuthFile:   authFile,
			Timeout:    5 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer c.Close()

		var result map[string]string
		if err := c.Call(context.Background(), "secure", nil, &result); err != nil {
			t.Fatalf("Call: %v", err)
		}
		if result["data"] != "secret" {
			t.Errorf("expected secret, got %s", result["data"])
		}
	})

	t.Run("without auth", func(t *testing.T) {
		c, err := NewClient(ClientConfig{
			TCPAddress: tcpAddr,
			Timeout:    5 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer c.Close()

		var result any
		err = c.Call(context.Background(), "secure", nil, &result)
		if err == nil {
			t.Fatal("expected error without auth")
		}
	})
}
