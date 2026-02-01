package core

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// testNodeCounter is used to generate unique node names for tests.
var testNodeCounter atomic.Uint64

// testConfig creates a test configuration with a unique node name.
// Each call generates a name like "test-node-1", "test-node-2", etc.
// This prevents "duplicate destination" errors when multiple tests run sequentially,
// as each I2P session needs a unique identity.
func testConfig(t *testing.T) *Config {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Node.DataDir = t.TempDir()
	// Generate unique node name to avoid I2P destination conflicts
	cfg.Node.Name = fmt.Sprintf("test-node-%d", testNodeCounter.Add(1))
	// Disable RPC and Web for test isolation
	cfg.RPC.Enabled = false
	cfg.Web.Enabled = false

	return cfg
}

// cleanupNode properly stops a node and waits for SAM session cleanup.
// This prevents "session limit exceeded" errors when running multiple tests sequentially.
// I2P SAM has a limited number of concurrent sessions (typically 3-16), and sessions
// take time to fully close after Stop() is called.
func cleanupNode(t *testing.T, node *Node) {
	t.Helper()

	if node == nil {
		return
	}

	// Only cleanup if node is running
	if node.GetState() != StateRunning {
		return
	}

	// Stop the node with reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := node.Stop(ctx); err != nil {
		t.Logf("Warning: Stop failed during cleanup: %v", err)
	}

	// Wait for SAM session to fully close
	// This is necessary because SAM sessions don't close immediately
	// and we have a limited number of concurrent sessions available
	// I2P tunnel teardown can take 2-3 seconds
	time.Sleep(3 * time.Second)
}
