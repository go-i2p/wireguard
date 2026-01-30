package core

import (
	"context"
	"testing"
	"time"
)

func TestNewNode_RequiresConfig(t *testing.T) {
	_, err := NewNode(nil)
	if err == nil {
		t.Error("NewNode should error when config is nil")
	}
}

func TestNewNode_ValidatesConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.Name = "" // Invalid

	_, err := NewNode(cfg)
	if err == nil {
		t.Error("NewNode should error when config is invalid")
	}
}

func TestNewNode_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	if node.GetState() != StateInitial {
		t.Errorf("initial state should be StateInitial, got %s", node.GetState())
	}
}

func TestNewNode_WithCustomLogger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	// Logger is now configured via DEBUG_I2P environment variable
	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	if node == nil {
		t.Fatal("node should not be nil")
	}
}

func TestNode_StartAndStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()

	// Start the node
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if node.GetState() != StateRunning {
		t.Errorf("state after Start should be StateRunning, got %s", node.GetState())
	}

	// Stop the node
	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := node.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if node.GetState() != StateStopped {
		t.Errorf("state after Stop should be StateStopped, got %s", node.GetState())
	}
}

func TestNode_CannotStartTwice(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		node.Stop(stopCtx)
	}()

	// Second start should fail
	if err := node.Start(ctx); err == nil {
		t.Error("Second Start should fail")
	}
}

func TestNode_CannotStopWhenNotRunning(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Stop without starting should fail
	if err := node.Stop(ctx); err == nil {
		t.Error("Stop without Start should fail")
	}
}

func TestNode_RestartAfterStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()

	// First start/stop cycle
	if err := node.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := node.Stop(stopCtx); err != nil {
		cancel()
		t.Fatalf("First Stop failed: %v", err)
	}
	cancel()

	// Should be able to start again after stopping
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}

	stopCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := node.Stop(stopCtx); err != nil {
		t.Fatalf("Second Stop failed: %v", err)
	}
}

func TestNode_Config(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()
	cfg.Node.Name = "test-config-node"

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	gotCfg := node.Config()
	if gotCfg.Node.Name != cfg.Node.Name {
		t.Errorf("Config().Node.Name = %q, want %q", gotCfg.Node.Name, cfg.Node.Name)
	}
}

func TestNode_DoneChannel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	done := node.Done()

	// Stop in a goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		node.Stop(stopCtx)
	}()

	// Done channel should be closed after stop
	// Note: device close can take up to 3 seconds to timeout
	select {
	case <-done:
		// Success
	case <-time.After(6 * time.Second):
		t.Error("Done channel was not closed after Stop")
	}
}

func TestNodeState_String(t *testing.T) {
	tests := []struct {
		state NodeState
		want  string
	}{
		{StateInitial, "initial"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopping, "stopping"},
		{StateStopped, "stopped"},
		{NodeState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("NodeState.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNode_GetConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()
	cfg.Node.Name = "test-node"

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	// Test getting specific keys
	tests := []struct {
		key  string
		want any
	}{
		{"node.name", "test-node"},
		{"rpc.enabled", true},
		{"mesh.max_peers", DefaultMaxPeers},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := node.GetConfig(tt.key)
			if err != nil {
				t.Fatalf("GetConfig(%q) error = %v", tt.key, err)
			}
			if got != tt.want {
				t.Errorf("GetConfig(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}

	// Test getting unknown key
	_, err = node.GetConfig("unknown.key")
	if err == nil {
		t.Error("GetConfig should error for unknown key")
	}

	// Test getting entire config
	cfg2, err := node.GetConfig("")
	if err != nil {
		t.Fatalf("GetConfig(\"\") error = %v", err)
	}
	if cfg2 == nil {
		t.Error("GetConfig(\"\") should return config")
	}
}

func TestNode_SetConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	// Test setting node.name
	oldVal, err := node.SetConfig("node.name", "new-name")
	if err != nil {
		t.Fatalf("SetConfig error = %v", err)
	}
	if oldVal != "my-node" {
		t.Errorf("SetConfig old value = %v, want 'my-node'", oldVal)
	}

	newVal, _ := node.GetConfig("node.name")
	if newVal != "new-name" {
		t.Errorf("GetConfig after SetConfig = %v, want 'new-name'", newVal)
	}

	// Test setting mesh.max_peers with int
	_, err = node.SetConfig("mesh.max_peers", 100)
	if err != nil {
		t.Fatalf("SetConfig mesh.max_peers error = %v", err)
	}
	maxPeers, _ := node.GetConfig("mesh.max_peers")
	if maxPeers != 100 {
		t.Errorf("mesh.max_peers = %v, want 100", maxPeers)
	}

	// Test setting read-only config
	_, err = node.SetConfig("node.data_dir", "/tmp/new")
	if err == nil {
		t.Error("SetConfig should error for read-only key")
	}

	// Test setting unknown key
	_, err = node.SetConfig("unknown.key", "value")
	if err == nil {
		t.Error("SetConfig should error for unknown key")
	}

	// Test setting with wrong type
	_, err = node.SetConfig("node.name", 123)
	if err == nil {
		t.Error("SetConfig should error for wrong type")
	}
}

func TestNode_PeerCount(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	// Before start, peer count should be 0
	if count := node.PeerCount(); count != 0 {
		t.Errorf("PeerCount before start = %d, want 0", count)
	}
}

func TestNode_I2PAddress(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	// Before start, I2P address should be empty
	if addr := node.I2PAddress(); addr != "" {
		t.Errorf("I2PAddress before start = %q, want empty", addr)
	}
}
