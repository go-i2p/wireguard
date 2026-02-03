package core

import (
	"os"
	"testing"
)

func TestNewExitNode(t *testing.T) {
	tests := []struct {
		name    string
		config  ExitNodeConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ExitNodeConfig{
				Enabled:         true,
				PublicInterface: "eth0",
			},
			wantErr: false,
		},
		{
			name: "disabled config",
			config: ExitNodeConfig{
				Enabled:         false,
				PublicInterface: "eth0",
			},
			wantErr: true,
		},
		{
			name: "missing public interface",
			config: ExitNodeConfig{
				Enabled:         true,
				PublicInterface: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExitNode(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExitNode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExitNode_IsActive(t *testing.T) {
	// Skip if not root (can't test iptables/sysctl without root)
	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "lo", // Use loopback for testing
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() error = %v", err)
	}

	// Should not be active initially
	if exitNode.IsActive() {
		t.Error("IsActive() = true, want false before Start()")
	}

	// Start the exit node
	if err := exitNode.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer exitNode.Stop()

	// Should be active after start
	if !exitNode.IsActive() {
		t.Error("IsActive() = false, want true after Start()")
	}

	// Stop the exit node
	if err := exitNode.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Should not be active after stop
	if exitNode.IsActive() {
		t.Error("IsActive() = true, want false after Stop()")
	}
}

func TestExitNode_StartStop(t *testing.T) {
	// Skip if not root - requires elevated privileges for firewall and sysctl operations
	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "lo",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() error = %v", err)
	}

	// Test starting
	if err := exitNode.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify node is active
	if !exitNode.IsActive() {
		t.Error("IsActive() = false, want true after Start()")
	}

	// Test stopping
	if err := exitNode.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify node is inactive
	if exitNode.IsActive() {
		t.Error("IsActive() = true, want false after Stop()")
	}

	// Test multiple stops (should not error)
	if err := exitNode.Stop(); err != nil {
		t.Errorf("Stop() on already stopped node error = %v, want nil", err)
	}
}

func TestExitNode_DoubleStart(t *testing.T) {
	// Skip if not root
	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "lo",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() error = %v", err)
	}

	// First start should succeed
	if err := exitNode.Start(); err != nil {
		t.Fatalf("First Start() error = %v", err)
	}
	defer exitNode.Stop()

	// Second start should fail
	if err := exitNode.Start(); err == nil {
		t.Error("Second Start() error = nil, want error for double start")
	}
}

// Platform-specific unit tests (setSysctl, getSysctl, runIPTables) have been removed.
// These functions are now internal to platform-specific implementations.
// Cross-platform behavior is tested via integration tests (TestExitNode_StartStop).

// TestExitNode_RestoreIPForwardState removed - forwarding state management is now internal to platform implementations
// Restoration behavior is tested indirectly via Start/Stop tests

// TestExitNode_CommandNotAvailable tests behavior when required commands are missing.
func TestExitNode_CommandNotAvailable(t *testing.T) {
	// Temporarily modify PATH to simulate missing commands
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set PATH to empty
	os.Setenv("PATH", "/nonexistent/path")

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
	}

	_, err := NewExitNode(config)
	if err == nil {
		t.Error("NewExitNode() with missing commands error = nil, want error")
	}
}

// TestExitNode_IPForwardingFailure tests behavior when IP forwarding fails.
func TestExitNode_IPForwardingFailure(t *testing.T) {
	// This test verifies error handling, not actual failure
	// (we can't easily simulate sysctl failure without mocking)
	if os.Geteuid() == 0 {
		t.Skip("Skipping test: would modify system state")
	}

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "eth0",
	}

	exitNode, _ := NewExitNode(config)
	if exitNode == nil {
		t.Skip("NewExitNode() failed, can't test IP forwarding failure")
	}

	// Attempt to start without root (should fail)
	err := exitNode.Start()
	if err == nil {
		t.Error("Start() without root error = nil, want error")
	}
}

// BenchmarkExitNode_StartStop benchmarks the start/stop cycle.
func BenchmarkExitNode_StartStop(b *testing.B) {
	if os.Geteuid() != 0 {
		b.Skip("Skipping benchmark: requires root privileges")
	}

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "lo",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		b.Fatalf("NewExitNode() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exitNode.Start(); err != nil {
			b.Fatalf("Start() error = %v", err)
		}
		if err := exitNode.Stop(); err != nil {
			b.Fatalf("Stop() error = %v", err)
		}
	}
}

// TestExitNode_ConcurrentAccess tests thread-safety of IsActive.
func TestExitNode_ConcurrentAccess(t *testing.T) {
	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "lo",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Skipf("NewExitNode() error = %v, skipping test (may require system tools)", err)
	}

	// Test concurrent IsActive calls
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = exitNode.IsActive()
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// Platform-specific helper functions removed - no longer needed with interface-based design

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
