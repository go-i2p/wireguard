// Package core provides the main orchestration logic for an I2P-based
// WireGuard mesh VPN node. It coordinates identity management, peer discovery,
// gossip protocols, and user interfaces.
package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Default configuration values.
// Note: DefaultTunnelLength is set to 1 because this mesh VPN operates on a
// trusted peer model where all nodes are considered trusted members. Lower
// tunnel length provides faster, more reliable connectivity. Use higher values
// (2-3) if anonymity between mesh peers is required.
const (
	DefaultSAMAddress        = "127.0.0.1:7656"
	DefaultTunnelLength      = 1
	DefaultTunnelSubnet      = "10.42.0.0/16"
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultPeerTimeout       = 5 * time.Minute
	DefaultMaxPeers          = 50
	DefaultShutdownTimeout   = 5 * time.Second
	DefaultDrainTimeout      = 10 * time.Second
	DefaultRPCSocket         = "rpc.sock"
	DefaultWebListen         = "127.0.0.1:8080"
)

// Config holds all configuration for an i2plan node.
type Config struct {
	Node NodeConfig `toml:"node"`
	I2P  I2PConfig  `toml:"i2p"`
	Mesh MeshConfig `toml:"mesh"`
	RPC  RPCConfig  `toml:"rpc"`
	Web  WebConfig  `toml:"web"`
}

// NodeConfig contains basic node identification settings.
type NodeConfig struct {
	// Name is a human-readable identifier for this node
	Name string `toml:"name"`
	// DataDir is the directory where persistent data is stored
	DataDir string `toml:"data_dir"`
}

// I2PConfig contains I2P transport settings.
type I2PConfig struct {
	// SAMAddress is the SAM bridge address (host:port)
	SAMAddress string `toml:"sam_address"`
	// TunnelLength is the number of hops for I2P tunnels (0-7).
	// Lower values are faster but provide less anonymity.
	// Default is 1 for trusted mesh networks; use 2-3 for untrusted peers.
	TunnelLength int `toml:"tunnel_length"`
}

// MeshConfig contains mesh network settings.
type MeshConfig struct {
	// TunnelSubnet is the IP range used for mesh IPs (e.g., "10.42.0.0/16")
	TunnelSubnet string `toml:"tunnel_subnet"`
	// HeartbeatInterval is how often to announce presence to peers
	HeartbeatInterval time.Duration `toml:"heartbeat_interval"`
	// PeerTimeout is how long before a peer is considered stale
	PeerTimeout time.Duration `toml:"peer_timeout"`
	// MaxPeers is the maximum number of peers to maintain connections with
	MaxPeers int `toml:"max_peers"`
	// ShutdownTimeout is the maximum time to wait for graceful device shutdown.
	// In production environments with slow I2P sessions, you may need to increase this.
	ShutdownTimeout time.Duration `toml:"shutdown_timeout"`
	// DrainTimeout is the maximum time to wait for in-flight requests to complete
	// during shutdown. This applies to RPC and Web servers.
	DrainTimeout time.Duration `toml:"drain_timeout"`
}

// RPCConfig contains RPC server settings.
type RPCConfig struct {
	// Enabled controls whether the RPC server is started
	Enabled bool `toml:"enabled"`
	// Socket is the path to the Unix socket for RPC (relative to DataDir)
	Socket string `toml:"socket"`
	// TCPAddress is an optional TCP address for RPC (e.g., "127.0.0.1:9090")
	TCPAddress string `toml:"tcp_address,omitempty"`
}

// WebConfig contains web UI settings.
type WebConfig struct {
	// Enabled controls whether the web UI is started
	Enabled bool `toml:"enabled"`
	// Listen is the address to bind the web server to
	Listen string `toml:"listen"`
}

// Note: The TUI runs as a separate client connecting via RPC.
// Use 'i2plan tui' to launch it - no configuration needed.

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".i2plan")

	return &Config{
		Node: NodeConfig{
			Name:    "my-node",
			DataDir: dataDir,
		},
		I2P: I2PConfig{
			SAMAddress:   DefaultSAMAddress,
			TunnelLength: DefaultTunnelLength,
		},
		Mesh: MeshConfig{
			TunnelSubnet:      DefaultTunnelSubnet,
			HeartbeatInterval: DefaultHeartbeatInterval,
			PeerTimeout:       DefaultPeerTimeout,
			MaxPeers:          DefaultMaxPeers,
			ShutdownTimeout:   DefaultShutdownTimeout,
			DrainTimeout:      DefaultDrainTimeout,
		},
		RPC: RPCConfig{
			Enabled: true,
			Socket:  DefaultRPCSocket,
		},
		Web: WebConfig{
			Enabled: true,
			Listen:  DefaultWebListen,
		},
	}
}

// LoadConfig reads configuration from a TOML file.
// If the file doesn't exist, it returns the default configuration.
// Environment variables with the I2PLAN_ prefix override configuration values.
//
// Environment variable mapping:
//   - I2PLAN_NODE_NAME -> Node.Name
//   - I2PLAN_DATA_DIR -> Node.DataDir
//   - I2PLAN_SAM_ADDRESS -> I2P.SAMAddress
//   - I2PLAN_TUNNEL_LENGTH -> I2P.TunnelLength
//   - I2PLAN_TUNNEL_SUBNET -> Mesh.TunnelSubnet
//   - I2PLAN_HEARTBEAT_INTERVAL -> Mesh.HeartbeatInterval (seconds)
//   - I2PLAN_PEER_TIMEOUT -> Mesh.PeerTimeout (seconds)
//   - I2PLAN_MAX_PEERS -> Mesh.MaxPeers
//   - I2PLAN_SHUTDOWN_TIMEOUT -> Mesh.ShutdownTimeout (seconds)
//   - I2PLAN_DRAIN_TIMEOUT -> Mesh.DrainTimeout (seconds)
//   - I2PLAN_RPC_ENABLED -> RPC.Enabled (true/false)
//   - I2PLAN_RPC_SOCKET -> RPC.Socket
//   - I2PLAN_RPC_TCP_ADDRESS -> RPC.TCPAddress
//   - I2PLAN_WEB_ENABLED -> Web.Enabled (true/false)
//   - I2PLAN_WEB_LISTEN -> Web.Listen
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Apply env overrides even if config file doesn't exist
			applyEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Apply environment variable overrides after loading from file
	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// SaveConfig writes the configuration to a TOML file.
// It creates the parent directory if it doesn't exist.
func SaveConfig(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// Validate checks the configuration for errors.
// It validates node, I2P, and mesh configuration sections.
func (c *Config) Validate() error {
	if err := c.validateNodeConfig(); err != nil {
		return err
	}
	if err := c.validateI2PConfig(); err != nil {
		return err
	}
	return c.validateMeshConfig()
}

// validateNodeConfig checks the node configuration section for errors.
func (c *Config) validateNodeConfig() error {
	if c.Node.Name == "" {
		return errors.New("node.name is required")
	}
	if c.Node.DataDir == "" {
		return errors.New("node.data_dir is required")
	}
	return nil
}

// validateI2PConfig checks the I2P configuration section for errors.
func (c *Config) validateI2PConfig() error {
	if c.I2P.SAMAddress == "" {
		return errors.New("i2p.sam_address is required")
	}
	if c.I2P.TunnelLength < 0 || c.I2P.TunnelLength > 7 {
		return errors.New("i2p.tunnel_length must be between 0 and 7")
	}
	return nil
}

// validateMeshConfig checks the mesh configuration section for errors.
func (c *Config) validateMeshConfig() error {
	if c.Mesh.TunnelSubnet == "" {
		return errors.New("mesh.tunnel_subnet is required")
	}
	if c.Mesh.MaxPeers < 1 {
		return errors.New("mesh.max_peers must be at least 1")
	}
	if c.Mesh.ShutdownTimeout < time.Second {
		return errors.New("mesh.shutdown_timeout must be at least 1 second")
	}
	return nil
}

// DataPath returns an absolute path within the data directory.
func (c *Config) DataPath(elem ...string) string {
	parts := append([]string{c.Node.DataDir}, elem...)
	return filepath.Join(parts...)
}

// EnsureDataDir creates the data directory if it doesn't exist.
func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.Node.DataDir, 0o700)
}

// applyEnvOverrides applies environment variable overrides to the configuration.
// Environment variables with the I2PLAN_ prefix take precedence over file-based config.
func applyEnvOverrides(cfg *Config) {
	// Node configuration
	if v := os.Getenv("I2PLAN_NODE_NAME"); v != "" {
		cfg.Node.Name = v
	}
	if v := os.Getenv("I2PLAN_DATA_DIR"); v != "" {
		cfg.Node.DataDir = v
	}

	// I2P configuration
	if v := os.Getenv("I2PLAN_SAM_ADDRESS"); v != "" {
		cfg.I2P.SAMAddress = v
	}
	if v := os.Getenv("I2PLAN_TUNNEL_LENGTH"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.I2P.TunnelLength = i
		}
	}

	// Mesh configuration
	if v := os.Getenv("I2PLAN_TUNNEL_SUBNET"); v != "" {
		cfg.Mesh.TunnelSubnet = v
	}
	if v := os.Getenv("I2PLAN_HEARTBEAT_INTERVAL"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Mesh.HeartbeatInterval = time.Duration(i) * time.Second
		}
	}
	if v := os.Getenv("I2PLAN_PEER_TIMEOUT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Mesh.PeerTimeout = time.Duration(i) * time.Second
		}
	}
	if v := os.Getenv("I2PLAN_MAX_PEERS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Mesh.MaxPeers = i
		}
	}
	if v := os.Getenv("I2PLAN_SHUTDOWN_TIMEOUT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Mesh.ShutdownTimeout = time.Duration(i) * time.Second
		}
	}
	if v := os.Getenv("I2PLAN_DRAIN_TIMEOUT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Mesh.DrainTimeout = time.Duration(i) * time.Second
		}
	}

	// RPC configuration
	if v := os.Getenv("I2PLAN_RPC_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.RPC.Enabled = b
		}
	}
	if v := os.Getenv("I2PLAN_RPC_SOCKET"); v != "" {
		cfg.RPC.Socket = v
	}
	if v := os.Getenv("I2PLAN_RPC_TCP_ADDRESS"); v != "" {
		cfg.RPC.TCPAddress = v
	}

	// Web configuration
	if v := os.Getenv("I2PLAN_WEB_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Web.Enabled = b
		}
	}
	if v := os.Getenv("I2PLAN_WEB_LISTEN"); v != "" {
		cfg.Web.Listen = v
	}
}
