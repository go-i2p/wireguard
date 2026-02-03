package embedded

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
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

// verifySAMSessionAvailable checks if a session name is available by attempting to create
// a temporary session and immediately destroying it. This helps detect hanging sessions.
func verifySAMSessionAvailable(sessionName, samAddress string) error {
	conn, err := net.DialTimeout("tcp", samAddress, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to SAM: %w", err)
	}
	defer conn.Close()

	// SAM handshake
	_, err = conn.Write([]byte("HELLO VERSION MIN=3.0 MAX=3.3\n"))
	if err != nil {
		return fmt.Errorf("failed to send HELLO: %w", err)
	}

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read HELLO response: %w", err)
	}

	response := string(buffer[:n])
	if !strings.Contains(response, "RESULT=OK") {
		return fmt.Errorf("SAM handshake failed: %s", response)
	}

	// Try to create a temporary session to check availability
	sessionCmd := fmt.Sprintf("SESSION CREATE STYLE=DATAGRAM ID=%s DESTINATION=TRANSIENT\n", sessionName)
	_, err = conn.Write([]byte(sessionCmd))
	if err != nil {
		return fmt.Errorf("failed to send session create: %w", err)
	}

	n, err = conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read session create response: %w", err)
	}

	response = string(buffer[:n])
	if strings.Contains(response, "already exists") {
		return fmt.Errorf("session name %s is not available", sessionName)
	}

	// If session was created, immediately destroy it
	if strings.Contains(response, "RESULT=OK") {
		_, _ = conn.Write([]byte("SESSION REMOVE\n"))
		conn.Read(buffer) // Read response but ignore errors
	}

	return nil
}

// testConfig creates a test configuration with a unique node name.
// Each call generates a name with counter, timestamp, PID, and random component.
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

	// Generate unique session name with multiple sources of entropy
	// Try up to 3 times if there are any conflicts (shouldn't happen with this level of uniqueness)
	var cfg Config
	for attempt := 0; attempt < 3; attempt++ {
		timestamp := time.Now().UnixNano() / 1000 // microseconds
		pid := os.Getpid()
		random := rand.Intn(999999) // 6-digit random number
		counter := testVPNCounter.Add(1)

		sessionName := fmt.Sprintf("test-vpn-%d-%d-%d-%d", counter, timestamp, pid, random)

		cfg = Config{
			NodeName:     sessionName,
			DataDir:      t.TempDir(),
			SAMAddress:   "127.0.0.1:7656",
			TunnelSubnet: "10.79.0.0/16",
		}

		// Verify the session name is available
		if err := verifySAMSessionAvailable(sessionName, cfg.SAMAddress); err != nil {
			t.Logf("Session name %s not available (attempt %d): %v", sessionName, attempt+1, err)
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond) // Increasing delay
				continue
			}
			// If all attempts failed, still try to use it (maybe verification is wrong)
			t.Logf("Using session name %s despite verification failure", sessionName)
		}
		break
	}

	t.Logf("Generated test config with NodeName: %s", cfg.NodeName)
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
