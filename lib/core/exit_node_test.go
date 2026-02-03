package core

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNewExitNode(t *testing.T) {
	// Check if iptables and sysctl are available
	if !hasIPTables() || !hasSysctl() {
		t.Skip("Skipping test: requires iptables and sysctl")
	}

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

	// Test starting
	if err := exitNode.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify IP forwarding is enabled
	value, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("getSysctl() error = %v", err)
	}
	if strings.TrimSpace(value) != "1" {
		t.Errorf("IP forwarding = %q, want \"1\"", strings.TrimSpace(value))
	}

	// Test stopping
	if err := exitNode.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
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

func TestCheckCommandAvailable(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "sh exists",
			command: "sh",
			wantErr: false,
		},
		{
			name:    "nonexistent command",
			command: "this-command-does-not-exist-12345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkCommandAvailable(tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkCommandAvailable(%q) error = %v, wantErr %v", tt.command, err, tt.wantErr)
			}
		})
	}
}

func TestSetSysctl(t *testing.T) {
	// Skip if not root
	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}

	// Get original value
	original, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("getSysctl() error = %v", err)
	}
	original = strings.TrimSpace(original)

	// Set to 1
	if err := setSysctl("net.ipv4.ip_forward", "1"); err != nil {
		t.Fatalf("setSysctl() error = %v", err)
	}

	// Verify it was set
	value, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("getSysctl() error = %v", err)
	}
	if strings.TrimSpace(value) != "1" {
		t.Errorf("getSysctl() = %q, want \"1\"", strings.TrimSpace(value))
	}

	// Restore original value
	if err := setSysctl("net.ipv4.ip_forward", original); err != nil {
		t.Fatalf("setSysctl() restore error = %v", err)
	}
}

func TestGetSysctl(t *testing.T) {
	// Check if sysctl is available
	if !hasSysctl() {
		t.Skip("Skipping test: requires sysctl")
	}

	// This test doesn't require root, just checks if sysctl works
	value, err := getSysctl("kernel.hostname")
	if err != nil {
		t.Fatalf("getSysctl() error = %v", err)
	}
	if value == "" {
		t.Error("getSysctl() returned empty hostname")
	}
}

func TestGetSysctl_Nonexistent(t *testing.T) {
	_, err := getSysctl("nonexistent.param.12345")
	if err == nil {
		t.Error("getSysctl() for nonexistent param error = nil, want error")
	}
}

func TestRunIPTables_InvalidRule(t *testing.T) {
	// Skip if not root
	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}

	// Invalid rule should fail
	err := runIPTables("-X NONEXISTENT_CHAIN")
	if err == nil {
		t.Error("runIPTables() with invalid rule error = nil, want error")
	}
}

func TestExitNode_NATSetupAndTeardown(t *testing.T) {
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

	// Setup NAT (this requires IP forwarding to be enabled first)
	original, _ := getSysctl("net.ipv4.ip_forward")
	_ = setSysctl("net.ipv4.ip_forward", "1")
	defer setSysctl("net.ipv4.ip_forward", strings.TrimSpace(original))

	if err := exitNode.setupNAT(); err != nil {
		t.Fatalf("setupNAT() error = %v", err)
	}

	// Verify rules were tracked
	if len(exitNode.iptablesRules) == 0 {
		t.Error("setupNAT() did not track any iptables rules")
	}

	// Teardown NAT
	if err := exitNode.teardownNAT(); err != nil {
		t.Fatalf("teardownNAT() error = %v", err)
	}

	// Verify rules were cleared
	if len(exitNode.iptablesRules) != 0 {
		t.Error("teardownNAT() did not clear iptables rules")
	}
}

func TestExitNode_RestoreIPForwardState(t *testing.T) {
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

	// Get original state
	original, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("getSysctl() error = %v", err)
	}
	original = strings.TrimSpace(original)

	// Start exit node (enables forwarding)
	if err := exitNode.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Stop exit node (should restore state)
	if err := exitNode.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify state was restored
	restored, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("getSysctl() error = %v", err)
	}
	if strings.TrimSpace(restored) != original {
		t.Errorf("IP forwarding state = %q, want original %q", strings.TrimSpace(restored), original)
	}
}

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
	// Check if iptables and sysctl are available
	if !hasIPTables() || !hasSysctl() {
		t.Skip("Skipping test: requires iptables and sysctl")
	}

	config := ExitNodeConfig{
		Enabled:         true,
		PublicInterface: "lo",
	}

	exitNode, err := NewExitNode(config)
	if err != nil {
		t.Fatalf("NewExitNode() error = %v", err)
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

// hasIPTables checks if iptables command is available.
func hasIPTables() bool {
	_, err := exec.LookPath("iptables")
	return err == nil
}

// hasSysctl checks if sysctl command is available.
func hasSysctl() bool {
	_, err := exec.LookPath("sysctl")
	return err == nil
}

func TestMain(m *testing.M) {
	// Check if required commands are available
	if !hasIPTables() {
		log.Warn("iptables not available, some tests will be skipped")
	}
	if !hasSysctl() {
		log.Warn("sysctl not available, some tests will be skipped")
	}

	os.Exit(m.Run())
}
