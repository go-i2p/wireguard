package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Node.Name == "" {
		t.Error("default config should have a node name")
	}
	if cfg.Node.DataDir == "" {
		t.Error("default config should have a data directory")
	}
	if cfg.I2P.SAMAddress == "" {
		t.Error("default config should have a SAM address")
	}
	if cfg.Mesh.TunnelSubnet == "" {
		t.Error("default config should have a tunnel subnet")
	}
	if cfg.Mesh.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("default config should have ShutdownTimeout=%v, got %v",
			DefaultShutdownTimeout, cfg.Mesh.ShutdownTimeout)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "empty node name",
			modify:  func(c *Config) { c.Node.Name = "" },
			wantErr: true,
		},
		{
			name:    "empty data dir",
			modify:  func(c *Config) { c.Node.DataDir = "" },
			wantErr: true,
		},
		{
			name:    "empty SAM address",
			modify:  func(c *Config) { c.I2P.SAMAddress = "" },
			wantErr: true,
		},
		{
			name:    "tunnel length too high",
			modify:  func(c *Config) { c.I2P.TunnelLength = 10 },
			wantErr: true,
		},
		{
			name:    "tunnel length negative",
			modify:  func(c *Config) { c.I2P.TunnelLength = -1 },
			wantErr: true,
		},
		{
			name:    "empty tunnel subnet",
			modify:  func(c *Config) { c.Mesh.TunnelSubnet = "" },
			wantErr: true,
		},
		{
			name:    "max peers zero",
			modify:  func(c *Config) { c.Mesh.MaxPeers = 0 },
			wantErr: true,
		},
		{
			name:    "shutdown timeout too short",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = 500 * time.Millisecond },
			wantErr: true,
		},
		{
			name:    "shutdown timeout zero",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = 0 },
			wantErr: true,
		},
		{
			name:    "shutdown timeout exactly 1 second",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = time.Second },
			wantErr: false,
		},
		{
			name:    "shutdown timeout extended for slow I2P",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = 30 * time.Second },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig_DefaultsWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.toml")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig should not error on missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig should return default config when file is missing")
	}
	if cfg.Node.Name == "" {
		t.Error("should have default node name")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Create a custom config
	original := DefaultConfig()
	original.Node.Name = "test-node"
	original.I2P.TunnelLength = 3
	original.Mesh.HeartbeatInterval = 60 * time.Second
	original.Mesh.ShutdownTimeout = 15 * time.Second // Custom timeout for slow I2P

	// Save it
	if err := SaveConfig(original, configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Load it back
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values
	if loaded.Node.Name != original.Node.Name {
		t.Errorf("node name mismatch: got %q, want %q", loaded.Node.Name, original.Node.Name)
	}
	if loaded.I2P.TunnelLength != original.I2P.TunnelLength {
		t.Errorf("tunnel length mismatch: got %d, want %d", loaded.I2P.TunnelLength, original.I2P.TunnelLength)
	}
	if loaded.Mesh.HeartbeatInterval != original.Mesh.HeartbeatInterval {
		t.Errorf("heartbeat interval mismatch: got %v, want %v", loaded.Mesh.HeartbeatInterval, original.Mesh.HeartbeatInterval)
	}
	if loaded.Mesh.ShutdownTimeout != original.Mesh.ShutdownTimeout {
		t.Errorf("shutdown timeout mismatch: got %v, want %v", loaded.Mesh.ShutdownTimeout, original.Mesh.ShutdownTimeout)
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.toml")

	// Write invalid TOML
	if err := os.WriteFile(configPath, []byte("this is not [valid toml"), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("LoadConfig should error on invalid TOML")
	}
}

func TestConfig_DataPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = "/home/user/.i2plan"

	path := cfg.DataPath("identity.json")
	expected := "/home/user/.i2plan/identity.json"
	if path != expected {
		t.Errorf("DataPath mismatch: got %q, want %q", path, expected)
	}

	path = cfg.DataPath("peers", "cache.json")
	expected = "/home/user/.i2plan/peers/cache.json"
	if path != expected {
		t.Errorf("DataPath mismatch: got %q, want %q", path, expected)
	}
}

func TestConfig_EnsureDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Node.DataDir = filepath.Join(tmpDir, "new", "data", "dir")

	if err := cfg.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir failed: %v", err)
	}

	info, err := os.Stat(cfg.Node.DataDir)
	if err != nil {
		t.Fatalf("data dir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("data path is not a directory")
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "new", "nested", "config.toml")

	cfg := DefaultConfig()
	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig failed to create nested directory: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}
