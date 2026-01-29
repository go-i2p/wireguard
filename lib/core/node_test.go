package core

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestNewNode_RequiresConfig(t *testing.T) {
	_, err := NewNode(nil, nil)
	if err == nil {
		t.Error("NewNode should error when config is nil")
	}
}

func TestNewNode_ValidatesConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.Name = "" // Invalid

	_, err := NewNode(cfg, nil)
	if err == nil {
		t.Error("NewNode should error when config is invalid")
	}
}

func TestNewNode_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	node, err := NewNode(cfg, nil)
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

	logger := slog.Default()
	node, err := NewNode(cfg, logger)
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

	node, err := NewNode(cfg, nil)
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
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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

	node, err := NewNode(cfg, nil)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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

	node, err := NewNode(cfg, nil)
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

	node, err := NewNode(cfg, nil)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx := context.Background()

	// First start/stop cycle
	if err := node.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := node.Stop(stopCtx); err != nil {
		cancel()
		t.Fatalf("First Stop failed: %v", err)
	}
	cancel()

	// Should be able to start again after stopping
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}

	stopCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := node.Stop(stopCtx); err != nil {
		t.Fatalf("Second Stop failed: %v", err)
	}
}

func TestNode_Config(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()
	cfg.Node.Name = "test-config-node"

	node, err := NewNode(cfg, nil)
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

	node, err := NewNode(cfg, nil)
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
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		node.Stop(stopCtx)
	}()

	// Done channel should be closed after stop
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
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
