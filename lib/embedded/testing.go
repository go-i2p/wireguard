package embedded

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testVPNCounter is used to generate unique VPN names for tests.
var testVPNCounter atomic.Uint64

// testMutex ensures only one test uses I2P SAM at a time to prevent session conflicts.
// I2P SAM has a limited number of concurrent sessions (typically 3-16), so running
// tests concurrently can cause "Session already exists" or "duplicate destination" errors.
var testMutex sync.Mutex

// testConfig creates a test configuration with a unique node name.
// Each call generates a name like "test-vpn-1-1738438800-12345", including PID and timestamp.
// This prevents "duplicate destination" errors when multiple tests run sequentially,
// as each I2P session needs a unique identity.
// It also acquires a global mutex to prevent concurrent SAM usage.
func testConfig(t testing.TB) Config {
	t.Helper()

	// Acquire global test mutex to prevent concurrent SAM usage
	testMutex.Lock()
	t.Cleanup(func() {
		testMutex.Unlock()
	})

	// Add microsecond timestamp and PID to ensure absolutely unique session names
	// This prevents any possibility of session name conflicts even in rapid test execution
	timestamp := time.Now().UnixNano() / 1000 // microseconds
	pid := os.Getpid()
	cfg := Config{
		NodeName:     fmt.Sprintf("test-vpn-%d-%d-%d", testVPNCounter.Add(1), timestamp, pid),
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
func cleanupVPN(t testing.TB, vpn *VPN) {
	t.Helper()

	if vpn == nil {
		return
	}

	// Stop the VPN with reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := vpn.Stop(ctx); err != nil {
		t.Logf("Warning: Stop failed during cleanup: %v", err)
	}

	// Ensure VPN has fully stopped before proceeding
	for i := 0; i < 10; i++ {
		if vpn.State() == StateStopped {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for SAM session to fully close
	// This is necessary because SAM sessions don't close immediately
	// and we have a limited number of concurrent sessions available
	// I2P tunnel teardown can take 5-8 seconds, especially under load
	// Extra time needed for session cleanup after Stop() - increased to 12 seconds for reliability
	time.Sleep(12 * time.Second)
}
