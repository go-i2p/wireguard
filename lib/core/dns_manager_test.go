package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewDNSManager tests the creation of a new DNS manager.
func TestNewDNSManager(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "valid creation",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm, err := NewDNSManager()

			if tt.wantErr {
				if err == nil {
					t.Error("NewDNSManager() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewDNSManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if dm == nil {
				t.Error("NewDNSManager() returned nil manager")
				return
			}

			if dm.resolvPath != "/etc/resolv.conf" {
				t.Errorf("NewDNSManager() resolvPath = %v, want /etc/resolv.conf", dm.resolvPath)
			}

			if dm.backupPath == "" {
				t.Error("NewDNSManager() backupPath is empty")
			}

			if dm.isManaged {
				t.Error("NewDNSManager() isManaged should be false initially")
			}
		})
	}
}

// TestDNSManager_UpdateAndRestore tests the full backup, update, and restore cycle.
func TestDNSManager_UpdateAndRestore(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()
	testResolvConf := filepath.Join(tmpDir, "resolv.conf")

	// Create initial resolv.conf
	originalContent := "nameserver 8.8.8.8\nnameserver 8.8.4.4\n"
	if err := os.WriteFile(testResolvConf, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test resolv.conf: %v", err)
	}

	// Create DNS manager
	dm, err := NewDNSManager()
	if err != nil {
		t.Fatalf("NewDNSManager() error = %v", err)
	}

	// Override paths for testing
	dm.resolvPath = testResolvConf
	dm.backupPath = filepath.Join(tmpDir, "resolv.conf.backup")

	// Test UpdateDNS
	newServers := []string{"1.1.1.1", "1.0.0.1"}
	if err := dm.UpdateDNS(newServers); err != nil {
		t.Fatalf("UpdateDNS() error = %v", err)
	}

	// Verify DNS was updated
	if !dm.IsManaged() {
		t.Error("IsManaged() should be true after UpdateDNS")
	}

	// Read updated content
	updatedContent, err := os.ReadFile(testResolvConf)
	if err != nil {
		t.Fatalf("Failed to read updated resolv.conf: %v", err)
	}

	updatedStr := string(updatedContent)
	if !strings.Contains(updatedStr, "nameserver 1.1.1.1") {
		t.Error("Updated resolv.conf should contain nameserver 1.1.1.1")
	}
	if !strings.Contains(updatedStr, "nameserver 1.0.0.1") {
		t.Error("Updated resolv.conf should contain nameserver 1.0.0.1")
	}
	if !strings.Contains(updatedStr, "Managed by i2plan") {
		t.Error("Updated resolv.conf should contain management comment")
	}

	// Test Restore
	if err := dm.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify restoration
	if dm.IsManaged() {
		t.Error("IsManaged() should be false after Restore")
	}

	restoredContent, err := os.ReadFile(testResolvConf)
	if err != nil {
		t.Fatalf("Failed to read restored resolv.conf: %v", err)
	}

	if string(restoredContent) != originalContent {
		t.Errorf("Restored content = %q, want %q", string(restoredContent), originalContent)
	}
}

// TestDNSManager_UpdateDNS_Validation tests DNS server validation.
func TestDNSManager_UpdateDNS_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	testResolvConf := filepath.Join(tmpDir, "resolv.conf")

	// Create initial resolv.conf
	if err := os.WriteFile(testResolvConf, []byte("nameserver 8.8.8.8\n"), 0644); err != nil {
		t.Fatalf("Failed to create test resolv.conf: %v", err)
	}

	// logger removed
	dm, err := NewDNSManager()
	if err != nil {
		t.Fatalf("NewDNSManager() error = %v", err)
	}

	dm.resolvPath = testResolvConf
	dm.backupPath = filepath.Join(tmpDir, "resolv.conf.backup")

	tests := []struct {
		name    string
		servers []string
		wantErr bool
	}{
		{
			name:    "empty servers list",
			servers: []string{},
			wantErr: true,
		},
		{
			name:    "valid IPv4 servers",
			servers: []string{"8.8.8.8", "1.1.1.1"},
			wantErr: false,
		},
		{
			name:    "valid IPv6 servers",
			servers: []string{"2001:4860:4860::8888", "2606:4700:4700::1111"},
			wantErr: false,
		},
		{
			name:    "invalid server with space",
			servers: []string{"8.8.8.8 invalid"},
			wantErr: true,
		},
		{
			name:    "invalid server with comment",
			servers: []string{"#8.8.8.8"},
			wantErr: true,
		},
		{
			name:    "empty string server",
			servers: []string{""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset managed state
			dm.isManaged = false

			err := dm.UpdateDNS(tt.servers)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateDNS() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Clean up if update succeeded
			if err == nil {
				_ = dm.Restore()
			}
		})
	}
}

// TestDNSManager_GetCurrentServers tests parsing of resolv.conf.
func TestDNSManager_GetCurrentServers(t *testing.T) {
	tmpDir := t.TempDir()
	testResolvConf := filepath.Join(tmpDir, "resolv.conf")

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "single server",
			content: "nameserver 8.8.8.8\n",
			want:    []string{"8.8.8.8"},
		},
		{
			name:    "multiple servers",
			content: "nameserver 8.8.8.8\nnameserver 1.1.1.1\n",
			want:    []string{"8.8.8.8", "1.1.1.1"},
		},
		{
			name:    "servers with comments",
			content: "# Comment\nnameserver 8.8.8.8\n# Another comment\nnameserver 1.1.1.1\n",
			want:    []string{"8.8.8.8", "1.1.1.1"},
		},
		{
			name:    "empty file",
			content: "",
			want:    []string{},
		},
		{
			name:    "no nameservers",
			content: "# Just comments\n# No servers\n",
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write test content
			if err := os.WriteFile(testResolvConf, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test resolv.conf: %v", err)
			}

			// logger removed
			dm, err := NewDNSManager()
			if err != nil {
				t.Fatalf("NewDNSManager() error = %v", err)
			}

			dm.resolvPath = testResolvConf

			got, err := dm.GetCurrentServers()
			if err != nil {
				t.Fatalf("GetCurrentServers() error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Errorf("GetCurrentServers() got %d servers, want %d", len(got), len(tt.want))
				return
			}

			for i, server := range got {
				if server != tt.want[i] {
					t.Errorf("GetCurrentServers() server[%d] = %v, want %v", i, server, tt.want[i])
				}
			}
		})
	}
}

// TestDNSManager_VerifyNoDNSLeak tests DNS leak verification.
func TestDNSManager_VerifyNoDNSLeak(t *testing.T) {
	tmpDir := t.TempDir()
	testResolvConf := filepath.Join(tmpDir, "resolv.conf")

	// logger removed
	dm, err := NewDNSManager()
	if err != nil {
		t.Fatalf("NewDNSManager() error = %v", err)
	}

	dm.resolvPath = testResolvConf

	tests := []struct {
		name            string
		currentServers  string
		expectedServers []string
		wantErr         bool
	}{
		{
			name:            "servers match",
			currentServers:  "nameserver 1.1.1.1\nnameserver 1.0.0.1\n",
			expectedServers: []string{"1.1.1.1", "1.0.0.1"},
			wantErr:         false,
		},
		{
			name:            "servers match - different order",
			currentServers:  "nameserver 1.0.0.1\nnameserver 1.1.1.1\n",
			expectedServers: []string{"1.1.1.1", "1.0.0.1"},
			wantErr:         false,
		},
		{
			name:            "servers don't match",
			currentServers:  "nameserver 8.8.8.8\nnameserver 8.8.4.4\n",
			expectedServers: []string{"1.1.1.1", "1.0.0.1"},
			wantErr:         true,
		},
		{
			name:            "no current servers",
			currentServers:  "",
			expectedServers: []string{"1.1.1.1"},
			wantErr:         true,
		},
		{
			name:            "wrong number of servers",
			currentServers:  "nameserver 1.1.1.1\n",
			expectedServers: []string{"1.1.1.1", "1.0.0.1"},
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write test content
			if err := os.WriteFile(testResolvConf, []byte(tt.currentServers), 0644); err != nil {
				t.Fatalf("Failed to write test resolv.conf: %v", err)
			}

			err := dm.VerifyNoDNSLeak(tt.expectedServers)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyNoDNSLeak() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDNSManager_Restore_NoBackup tests restore when no backup exists.
func TestDNSManager_Restore_NoBackup(t *testing.T) {
	tmpDir := t.TempDir()
	testResolvConf := filepath.Join(tmpDir, "resolv.conf")

	// Create initial resolv.conf
	if err := os.WriteFile(testResolvConf, []byte("nameserver 8.8.8.8\n"), 0644); err != nil {
		t.Fatalf("Failed to create test resolv.conf: %v", err)
	}

	// logger removed
	dm, err := NewDNSManager()
	if err != nil {
		t.Fatalf("NewDNSManager() error = %v", err)
	}

	dm.resolvPath = testResolvConf
	dm.backupPath = filepath.Join(tmpDir, "nonexistent.backup")
	dm.isManaged = true    // Simulate managed state
	dm.originalResolv = "" // No in-memory backup

	// Attempt restore should fail
	err = dm.Restore()
	if err == nil {
		t.Error("Restore() should fail when no backup exists")
	}
}

// TestDNSManager_ConcurrentAccess tests thread safety.
func TestDNSManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	testResolvConf := filepath.Join(tmpDir, "resolv.conf")

	// Create initial resolv.conf
	if err := os.WriteFile(testResolvConf, []byte("nameserver 8.8.8.8\n"), 0644); err != nil {
		t.Fatalf("Failed to create test resolv.conf: %v", err)
	}

	// logger removed
	dm, err := NewDNSManager()
	if err != nil {
		t.Fatalf("NewDNSManager() error = %v", err)
	}

	dm.resolvPath = testResolvConf
	dm.backupPath = filepath.Join(tmpDir, "resolv.conf.backup")

	// Run concurrent operations
	done := make(chan bool)

	// Goroutine 1: Multiple IsManaged calls
	go func() {
		for i := 0; i < 100; i++ {
			_ = dm.IsManaged()
		}
		done <- true
	}()

	// Goroutine 2: Multiple GetCurrentServers calls
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = dm.GetCurrentServers()
		}
		done <- true
	}()

	// Wait for completion
	<-done
	<-done

	// No assertion needed - test passes if no race detector issues
}

// TestIsValidDNSServer tests DNS server validation.
func TestIsValidDNSServer(t *testing.T) {
	tests := []struct {
		name   string
		server string
		want   bool
	}{
		{
			name:   "valid IPv4",
			server: "8.8.8.8",
			want:   true,
		},
		{
			name:   "valid IPv6",
			server: "2001:4860:4860::8888",
			want:   true,
		},
		{
			name:   "valid hostname",
			server: "dns.google",
			want:   true,
		},
		{
			name:   "empty string",
			server: "",
			want:   false,
		},
		{
			name:   "whitespace only",
			server: "   ",
			want:   false,
		},
		{
			name:   "contains space",
			server: "8.8.8.8 invalid",
			want:   false,
		},
		{
			name:   "contains newline",
			server: "8.8.8.8\n",
			want:   false,
		},
		{
			name:   "comment",
			server: "#8.8.8.8",
			want:   false,
		},
		{
			name:   "double slash",
			server: "//8.8.8.8",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidDNSServer(tt.server)
			if got != tt.want {
				t.Errorf("isValidDNSServer(%q) = %v, want %v", tt.server, got, tt.want)
			}
		})
	}
}

// TestServersMatch tests server list comparison.
func TestServersMatch(t *testing.T) {
	tests := []struct {
		name     string
		current  []string
		expected []string
		want     bool
	}{
		{
			name:     "exact match",
			current:  []string{"1.1.1.1", "1.0.0.1"},
			expected: []string{"1.1.1.1", "1.0.0.1"},
			want:     true,
		},
		{
			name:     "different order - match",
			current:  []string{"1.0.0.1", "1.1.1.1"},
			expected: []string{"1.1.1.1", "1.0.0.1"},
			want:     true,
		},
		{
			name:     "different servers",
			current:  []string{"8.8.8.8", "8.8.4.4"},
			expected: []string{"1.1.1.1", "1.0.0.1"},
			want:     false,
		},
		{
			name:     "different lengths",
			current:  []string{"1.1.1.1"},
			expected: []string{"1.1.1.1", "1.0.0.1"},
			want:     false,
		},
		{
			name:     "empty lists",
			current:  []string{},
			expected: []string{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serversMatch(tt.current, tt.expected)
			if got != tt.want {
				t.Errorf("serversMatch(%v, %v) = %v, want %v", tt.current, tt.expected, got, tt.want)
			}
		})
	}
}

// TestCopyFile tests file copying utility.
func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := "test content\n"
	if err := os.WriteFile(srcPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test copy to existing directory
	dstPath := filepath.Join(tmpDir, "dest.txt")
	if err := CopyFile(srcPath, dstPath); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	// Verify content
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(dstContent) != content {
		t.Errorf("CopyFile() content = %q, want %q", string(dstContent), content)
	}

	// Test copy to non-existing directory
	dstPath2 := filepath.Join(tmpDir, "subdir", "dest2.txt")
	if err := CopyFile(srcPath, dstPath2); err != nil {
		t.Fatalf("CopyFile() to new directory error = %v", err)
	}

	// Verify content
	dstContent2, err := os.ReadFile(dstPath2)
	if err != nil {
		t.Fatalf("Failed to read destination file in new directory: %v", err)
	}

	if string(dstContent2) != content {
		t.Errorf("CopyFile() to new directory content = %q, want %q", string(dstContent2), content)
	}

	// Test copy non-existing source
	if err := CopyFile(filepath.Join(tmpDir, "nonexistent.txt"), dstPath); err == nil {
		t.Error("CopyFile() should fail for non-existing source")
	}
}
