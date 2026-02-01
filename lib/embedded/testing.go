package embedded

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// testVPNCounter is used to generate unique VPN names for tests.
var testVPNCounter atomic.Uint64

// testConfig creates a test configuration with a unique node name.
// Each call generates a name like "test-vpn-1", "test-vpn-2", etc.
// This prevents "duplicate destination" errors when multiple tests run sequentially,
// as each I2P session needs a unique identity.
func testConfig(t *testing.T) Config {
	t.Helper()

	cfg := Config{
		NodeName:     fmt.Sprintf("test-vpn-%d", testVPNCounter.Add(1)),
		DataDir:      t.TempDir(),
		SAMAddress:   "127.0.0.1:7656",
		TunnelSubnet: "10.79.0.0/16",
	}

	return cfg
}

// cleanupVPN properly stops a VPN and waits for SAM session cleanup.
// This prevents "session limit exceeded" errors when running multiple tests sequentially.
// I2P SAM has a limited number of concurrent sessions (typically 3-16), and sessions
// take time to fully close after Stop() is called.
func cleanupVPN(t *testing.T, vpn *VPN) {
	t.Helper()

	if vpn == nil {
		return
	}

	// Stop the VPN with reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := vpn.Stop(ctx); err != nil {
		t.Logf("Warning: Stop failed during cleanup: %v", err)
	}

	// Wait for SAM session to fully close
	// This is necessary because SAM sessions don't close immediately
	// and we have a limited number of concurrent sessions available
	// I2P tunnel teardown can take 2-3 seconds
	time.Sleep(3 * time.Second)
}
