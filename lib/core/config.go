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
	Node       NodeConfig       `toml:"node"`
	I2P        I2PConfig        `toml:"i2p"`
	Mesh       MeshConfig       `toml:"mesh"`
	RPC        RPCConfig        `toml:"rpc"`
	Web        WebConfig        `toml:"web"`
	ExitNode   ExitNodeConfig   `toml:"exit_node"`
	ExitClient ClientExitConfig `toml:"exit_client"`
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

// ExitNodeConfig contains settings for acting as an exit node.
// An exit node forwards traffic from mesh clients to the public internet,
// acting as a gateway for other nodes in the mesh.
//
// SECURITY WARNING: Running an exit node forwards other users' traffic through
// your connection. Ensure you understand the legal and security implications.
// Use bandwidth limits and client allowlists to control resource usage.
//
// SYSTEM REQUIREMENTS: Exit nodes require CAP_NET_ADMIN or root privileges to:
//   - Enable IP forwarding (sysctl net.ipv4.ip_forward=1)
//   - Configure NAT/masquerading (iptables/nftables)
//   - Manage routing tables
type ExitNodeConfig struct {
	// Enabled controls whether this node acts as an exit node
	Enabled bool `toml:"enabled"`
	// PublicInterface is the network interface for outbound internet traffic (e.g., "eth0", "wlan0").
	// This interface is used for NAT/masquerading of mesh traffic.
	PublicInterface string `toml:"public_interface"`
	// AllowedClients lists peer public keys or names allowed to use this exit.
	// Empty list allows all mesh peers to use this exit node.
	AllowedClients []string `toml:"allowed_clients"`
	// BandwidthLimitMbps limits the maximum bandwidth in Mbps for exit traffic.
	// 0 means unlimited. Recommended to set a reasonable limit to prevent abuse.
	BandwidthLimitMbps int `toml:"bandwidth_limit_mbps"`
	// LogConnections enables logging of client connections and traffic patterns.
	// Useful for debugging and monitoring but may impact privacy.
	LogConnections bool `toml:"log_connections"`
	// UpstreamVPN specifies the VPN interface to route mesh traffic through (e.g., "wg-mullvad", "tun0").
	// Leave empty to route directly through PublicInterface.
	UpstreamVPN string `toml:"upstream_vpn"`
	// AutoDetect enables automatic detection of upstream VPN interfaces.
	// When true, the exit node will scan for active VPN connections and use the first available.
	AutoDetect bool `toml:"auto_detect_vpn"`
	// FallbackBehavior controls what happens when upstream VPN becomes unavailable.
	// "direct" routes traffic directly through PublicInterface (default).
	// "block" blocks all traffic until VPN reconnects (more privacy-focused).
	FallbackBehavior string `toml:"fallback_behavior"`
}

// ClientExitConfig contains settings for using another node as an exit.
// When enabled, this node routes all internet traffic through a designated
// exit node in the mesh, similar to a traditional VPN client.
type ClientExitConfig struct {
	// Enabled controls whether this node uses an exit node
	Enabled bool `toml:"enabled"`
	// ExitNodeID identifies the exit node by peer public key or name.
	// Leave empty to auto-select the best available exit node.
	ExitNodeID string `toml:"exit_node_id"`
	// PreferredRoute specifies a preferred routing option name (e.g., "mullvad-sweden", "direct").
	// If empty, the best route is auto-selected based on bandwidth and latency.
	// If specified but not available, falls back to other routes unless RequireVPN is true.
	PreferredRoute string `toml:"preferred_route"`
	// RequireVPN mandates that only exit nodes with upstream VPN connections can be used.
	// If true and no VPN-backed exits are available, connection fails rather than using direct routing.
	RequireVPN bool `toml:"require_vpn"`
	// KillSwitch blocks all non-mesh traffic if the exit node becomes unavailable.
	// Prevents traffic leaks but blocks internet access until exit reconnects.
	KillSwitch bool `toml:"kill_switch"`
	// DNSServers lists DNS servers to use via the exit node (e.g., ["1.1.1.1", "8.8.8.8"]).
	// Empty list uses exit node's default DNS. Prevents DNS leaks.
	DNSServers []string `toml:"dns_servers"`
	// ExcludeRoutes lists local subnets to bypass the exit node (e.g., ["192.168.1.0/24"]).
	// Useful for accessing local network resources (printers, file shares) while using exit mode.
	ExcludeRoutes []string `toml:"exclude_routes"`
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
		ExitNode: ExitNodeConfig{
			Enabled:            false,
			BandwidthLimitMbps: 0,
			LogConnections:     false,
			AutoDetect:         false,
			FallbackBehavior:   "direct",
		},
		ExitClient: ClientExitConfig{
			Enabled:    false,
			KillSwitch: false,
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
//   - I2PLAN_EXIT_NODE_ENABLED -> ExitNode.Enabled (true/false)
//   - I2PLAN_EXIT_NODE_PUBLIC_INTERFACE -> ExitNode.PublicInterface
//   - I2PLAN_EXIT_NODE_ALLOWED_CLIENTS -> ExitNode.AllowedClients (comma-separated)
//   - I2PLAN_EXIT_NODE_BANDWIDTH_LIMIT_MBPS -> ExitNode.BandwidthLimitMbps
//   - I2PLAN_EXIT_NODE_LOG_CONNECTIONS -> ExitNode.LogConnections (true/false)
//   - I2PLAN_EXIT_CLIENT_ENABLED -> ExitClient.Enabled (true/false)
//   - I2PLAN_EXIT_CLIENT_EXIT_NODE_ID -> ExitClient.ExitNodeID
//   - I2PLAN_EXIT_CLIENT_KILL_SWITCH -> ExitClient.KillSwitch (true/false)
//   - I2PLAN_EXIT_CLIENT_DNS_SERVERS -> ExitClient.DNSServers (comma-separated)
//   - I2PLAN_EXIT_CLIENT_EXCLUDE_ROUTES -> ExitClient.ExcludeRoutes (comma-separated)
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
	if err := c.validateMeshConfig(); err != nil {
		return err
	}
	if err := c.validateExitNodeConfig(); err != nil {
		return err
	}
	return c.validateExitClientConfig()
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

// validateExitNodeConfig checks the exit node configuration section for errors.
func (c *Config) validateExitNodeConfig() error {
	if !c.ExitNode.Enabled {
		return nil
	}

	if c.ExitNode.PublicInterface == "" {
		return errors.New("exit_node.public_interface is required when exit node is enabled")
	}

	if c.ExitNode.BandwidthLimitMbps < 0 {
		return errors.New("exit_node.bandwidth_limit_mbps must be non-negative")
	}

	return nil
}

// validateExitClientConfig checks the exit client configuration section for errors.
func (c *Config) validateExitClientConfig() error {
	if !c.ExitClient.Enabled {
		return nil
	}

	// Validate exclude routes are valid CIDR notation if provided
	for _, route := range c.ExitClient.ExcludeRoutes {
		if route == "" {
			return errors.New("exit_client.exclude_routes: empty route not allowed")
		}
		// Basic CIDR format check - more thorough validation happens at runtime
		if !containsCIDRNotation(route) {
			return fmt.Errorf("exit_client.exclude_routes: invalid CIDR format: %s", route)
		}
	}

	return nil
}

// containsCIDRNotation checks if a string looks like CIDR notation (contains /).
// More thorough validation is done by net.ParseCIDR at runtime.
func containsCIDRNotation(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
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
	applyNodeEnvOverrides(&cfg.Node)
	applyI2PEnvOverrides(&cfg.I2P)
	applyMeshEnvOverrides(&cfg.Mesh)
	applyRPCEnvOverrides(&cfg.RPC)
	applyWebEnvOverrides(&cfg.Web)
	applyExitNodeEnvOverrides(&cfg.ExitNode)
	applyExitClientEnvOverrides(&cfg.ExitClient)
}

// applyNodeEnvOverrides applies environment variable overrides to node configuration.
func applyNodeEnvOverrides(node *NodeConfig) {
	if v := os.Getenv("I2PLAN_NODE_NAME"); v != "" {
		node.Name = v
	}
	if v := os.Getenv("I2PLAN_DATA_DIR"); v != "" {
		node.DataDir = v
	}
}

// applyI2PEnvOverrides applies environment variable overrides to I2P configuration.
func applyI2PEnvOverrides(i2p *I2PConfig) {
	if v := os.Getenv("I2PLAN_SAM_ADDRESS"); v != "" {
		i2p.SAMAddress = v
	}
	applyIntEnv("I2PLAN_TUNNEL_LENGTH", &i2p.TunnelLength)
}

// applyMeshEnvOverrides applies environment variable overrides to mesh configuration.
func applyMeshEnvOverrides(mesh *MeshConfig) {
	if v := os.Getenv("I2PLAN_TUNNEL_SUBNET"); v != "" {
		mesh.TunnelSubnet = v
	}
	applyDurationEnv("I2PLAN_HEARTBEAT_INTERVAL", &mesh.HeartbeatInterval)
	applyDurationEnv("I2PLAN_PEER_TIMEOUT", &mesh.PeerTimeout)
	applyIntEnv("I2PLAN_MAX_PEERS", &mesh.MaxPeers)
	applyDurationEnv("I2PLAN_SHUTDOWN_TIMEOUT", &mesh.ShutdownTimeout)
	applyDurationEnv("I2PLAN_DRAIN_TIMEOUT", &mesh.DrainTimeout)
}

// applyRPCEnvOverrides applies environment variable overrides to RPC configuration.
func applyRPCEnvOverrides(rpc *RPCConfig) {
	applyBoolEnv("I2PLAN_RPC_ENABLED", &rpc.Enabled)
	if v := os.Getenv("I2PLAN_RPC_SOCKET"); v != "" {
		rpc.Socket = v
	}
	if v := os.Getenv("I2PLAN_RPC_TCP_ADDRESS"); v != "" {
		rpc.TCPAddress = v
	}
}

// applyWebEnvOverrides applies environment variable overrides to web configuration.
func applyWebEnvOverrides(web *WebConfig) {
	applyBoolEnv("I2PLAN_WEB_ENABLED", &web.Enabled)
	if v := os.Getenv("I2PLAN_WEB_LISTEN"); v != "" {
		web.Listen = v
	}
}

// applyIntEnv reads an integer environment variable and applies it to the target.
func applyIntEnv(key string, target *int) {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			*target = i
		}
	}
}

// applyDurationEnv reads a duration environment variable in seconds and applies it to the target.
func applyDurationEnv(key string, target *time.Duration) {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			*target = time.Duration(i) * time.Second
		}
	}
}

// applyBoolEnv reads a boolean environment variable and applies it to the target.
func applyBoolEnv(key string, target *bool) {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*target = b
		}
	}
}

// applyExitNodeEnvOverrides applies environment variable overrides to exit node configuration.
func applyExitNodeEnvOverrides(exitNode *ExitNodeConfig) {
	applyBoolEnv("I2PLAN_EXIT_NODE_ENABLED", &exitNode.Enabled)
	if v := os.Getenv("I2PLAN_EXIT_NODE_PUBLIC_INTERFACE"); v != "" {
		exitNode.PublicInterface = v
	}
	if v := os.Getenv("I2PLAN_EXIT_NODE_ALLOWED_CLIENTS"); v != "" {
		// Parse comma-separated list
		exitNode.AllowedClients = parseCommaSeparated(v)
	}
	applyIntEnv("I2PLAN_EXIT_NODE_BANDWIDTH_LIMIT_MBPS", &exitNode.BandwidthLimitMbps)
	applyBoolEnv("I2PLAN_EXIT_NODE_LOG_CONNECTIONS", &exitNode.LogConnections)
}

// applyExitClientEnvOverrides applies environment variable overrides to exit client configuration.
func applyExitClientEnvOverrides(exitClient *ClientExitConfig) {
	applyBoolEnv("I2PLAN_EXIT_CLIENT_ENABLED", &exitClient.Enabled)
	if v := os.Getenv("I2PLAN_EXIT_CLIENT_EXIT_NODE_ID"); v != "" {
		exitClient.ExitNodeID = v
	}
	applyBoolEnv("I2PLAN_EXIT_CLIENT_KILL_SWITCH", &exitClient.KillSwitch)
	if v := os.Getenv("I2PLAN_EXIT_CLIENT_DNS_SERVERS"); v != "" {
		exitClient.DNSServers = parseCommaSeparated(v)
	}
	if v := os.Getenv("I2PLAN_EXIT_CLIENT_EXCLUDE_ROUTES"); v != "" {
		exitClient.ExcludeRoutes = parseCommaSeparated(v)
	}
}

// parseCommaSeparated splits a comma-separated string into a slice, trimming whitespace.
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				// Trim whitespace manually to avoid importing strings
				part := s[start:i]
				part = trimSpace(part)
				if part != "" {
					parts = append(parts, part)
				}
			}
			start = i + 1
		}
	}
	// Handle last part
	if start < len(s) {
		part := s[start:]
		part = trimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// trimSpace removes leading and trailing whitespace from a string.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
