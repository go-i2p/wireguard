package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-i2p/wireguard/lib/identity"
	"github.com/go-i2p/wireguard/lib/mesh"
	"github.com/go-i2p/wireguard/lib/rpc"
	"github.com/go-i2p/wireguard/lib/transport"
	"github.com/go-i2p/wireguard/lib/web"
	"github.com/go-i2p/wireguard/version"
)

// NodeState represents the current state of the node.
type NodeState int

const (
	// StateInitial is the initial state before Start is called.
	StateInitial NodeState = iota
	// StateStarting means the node is in the process of starting.
	StateStarting
	// StateRunning means the node is fully operational.
	StateRunning
	// StateStopping means the node is shutting down.
	StateStopping
	// StateStopped means the node has been stopped.
	StateStopped
)

func (s NodeState) String() string {
	switch s {
	case StateInitial:
		return "initial"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Node is the main orchestrator for an i2plan mesh VPN node.
// It coordinates identity management, I2P transport, WireGuard device,
// gossip protocol, and user interfaces.
type Node struct {
	mu     sync.RWMutex
	config *Config
	logger *slog.Logger
	state  NodeState

	// cancel is used to signal shutdown to all goroutines
	cancel context.CancelFunc
	// done signals that the node has fully stopped
	done chan struct{}

	// startedAt tracks when the node started
	startedAt time.Time

	// Event callbacks for embedded API integration
	onStateChange func(oldState, newState NodeState)
	onError       func(err error, message string)

	// Core components
	identity    *identity.Identity
	inviteStore *identity.InviteStore
	trans       *transport.Transport
	sender      *transport.Sender
	device      *mesh.Device
	routing     *mesh.RoutingTable
	peers       *mesh.PeerManager
	gossip      *mesh.GossipEngine
	banList     *mesh.BanList

	// User interfaces
	rpcServer *rpc.Server
	webServer *web.Server

	// Derived values
	tunnelIP netip.Addr
}

// NewNode creates a new Node with the given configuration.
// The node is not started until Start() is called.
func NewNode(cfg *Config, logger *slog.Logger) (*Node, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Node{
		config: cfg,
		logger: logger.With("component", "node"),
		state:  StateInitial,
		done:   make(chan struct{}),
	}, nil
}

// Start initializes and starts all node components.
// This includes:
//   - Creating data directory
//   - Loading or generating identity
//   - Opening I2P transport
//   - Starting WireGuard device
//   - Starting gossip protocol
//   - Starting user interfaces (RPC, Web, TUI) if enabled
//
// Start blocks until the node is fully initialized or an error occurs.
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	if n.state != StateInitial && n.state != StateStopped {
		n.mu.Unlock()
		return fmt.Errorf("cannot start node in state %s", n.state)
	}
	oldState := n.state
	n.state = StateStarting
	n.done = make(chan struct{})
	n.mu.Unlock()

	n.emitStateChange(oldState, StateStarting)

	// Create a cancellable context for the node's lifetime
	nodeCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	n.logger.Info("starting node",
		"name", n.config.Node.Name,
		"data_dir", n.config.Node.DataDir,
	)

	// Ensure data directory exists
	if err := n.config.EnsureDataDir(); err != nil {
		n.transitionToStopped()
		n.emitError(err, "failed to create data directory")
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Phase 1: Load or generate identity
	if err := n.initIdentity(); err != nil {
		n.transitionToStopped()
		n.emitError(err, "failed to initialize identity")
		return err
	}

	// Phase 2: Open I2P transport and WireGuard device
	if err := n.initTransport(); err != nil {
		n.transitionToStopped()
		n.emitError(err, "failed to initialize transport")
		return err
	}

	if err := n.initDevice(); err != nil {
		n.cleanup()
		n.transitionToStopped()
		n.emitError(err, "failed to initialize device")
		return err
	}

	// Phase 3: Initialize mesh components and start gossip
	if err := n.initMesh(nodeCtx); err != nil {
		n.cleanup()
		n.transitionToStopped()
		n.emitError(err, "failed to initialize mesh")
		return err
	}

	// Phase 4: Start user interfaces (RPC, Web) if enabled
	if err := n.initInterfaces(nodeCtx); err != nil {
		n.cleanup()
		n.transitionToStopped()
		n.emitError(err, "failed to initialize interfaces")
		return err
	}

	n.mu.Lock()
	n.state = StateRunning
	n.startedAt = time.Now()
	n.mu.Unlock()

	n.emitStateChange(StateStarting, StateRunning)
	n.logger.Info("node started")

	// Start the main run loop in a goroutine
	go n.run(nodeCtx)

	return nil
}

// run is the main loop that runs until the context is cancelled.
func (n *Node) run(ctx context.Context) {
	defer close(n.done)

	<-ctx.Done()

	n.logger.Info("node shutting down")

	// Cleanup all components
	n.cleanup()

	n.mu.Lock()
	oldState := n.state
	n.state = StateStopped
	n.mu.Unlock()

	n.emitStateChange(oldState, StateStopped)
}

// Stop gracefully shuts down the node.
// It blocks until all components have stopped or the context is cancelled.
func (n *Node) Stop(ctx context.Context) error {
	n.mu.Lock()
	if n.state != StateRunning {
		n.mu.Unlock()
		return fmt.Errorf("cannot stop node in state %s", n.state)
	}
	n.state = StateStopping
	cancel := n.cancel
	n.mu.Unlock()

	n.emitStateChange(StateRunning, StateStopping)
	n.logger.Info("stopping node")

	// Signal all goroutines to stop
	if cancel != nil {
		cancel()
	}

	// Wait for the run loop to finish or timeout
	select {
	case <-n.done:
		n.logger.Info("node stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// transitionToStopped updates the state to stopped.
func (n *Node) transitionToStopped() {
	n.mu.Lock()
	n.state = StateStopped
	n.mu.Unlock()
}

// GetState returns the current state of the node (typed).
func (n *Node) GetState() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state
}

// State returns the current state as a string for RPC.
func (n *Node) State() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state.String()
}

// Config returns the node's configuration.
func (n *Node) Config() *Config {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config
}

// Done returns a channel that is closed when the node has stopped.
func (n *Node) Done() <-chan struct{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.done
}

// StartedAt returns when the node was started.
// Returns zero time if not started.
func (n *Node) StartedAt() time.Time {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.startedAt
}

// Uptime returns how long the node has been running.
// Returns zero if not running.
func (n *Node) Uptime() time.Duration {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.startedAt.IsZero() || n.state != StateRunning {
		return 0
	}
	return time.Since(n.startedAt)
}

// SetOnStateChange sets a callback for state changes.
// The callback is invoked synchronously during state transitions.
func (n *Node) SetOnStateChange(callback func(oldState, newState NodeState)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onStateChange = callback
}

// SetOnError sets a callback for error events.
// The callback is invoked when recoverable errors occur.
func (n *Node) SetOnError(callback func(err error, message string)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onError = callback
}

// emitStateChange notifies the state change callback if set.
func (n *Node) emitStateChange(oldState, newState NodeState) {
	n.mu.RLock()
	callback := n.onStateChange
	n.mu.RUnlock()

	if callback != nil {
		callback(oldState, newState)
	}
}

// emitError notifies the error callback if set.
func (n *Node) emitError(err error, message string) {
	n.mu.RLock()
	callback := n.onError
	n.mu.RUnlock()

	if callback != nil {
		callback(err, message)
	}
}

// initIdentity loads an existing identity or generates a new one.
func (n *Node) initIdentity() error {
	identityPath := filepath.Join(n.config.Node.DataDir, identity.IdentityFileName)

	n.logger.Info("loading identity", "path", identityPath)

	id, err := identity.LoadIdentity(identityPath)
	if err != nil {
		return fmt.Errorf("loading identity: %w", err)
	}

	if id == nil {
		// No existing identity, generate a new one
		n.logger.Info("generating new identity")
		id, err = identity.NewIdentity()
		if err != nil {
			return fmt.Errorf("generating identity: %w", err)
		}

		if err := id.Save(identityPath); err != nil {
			return fmt.Errorf("saving identity: %w", err)
		}
	}

	n.identity = id
	n.logger.Info("identity loaded",
		"node_id", id.NodeID(),
		"public_key", id.PublicKey().String()[:16]+"...",
	)

	// Load or create invite store
	inviteStorePath := filepath.Join(n.config.Node.DataDir, "invites.json")
	n.inviteStore, err = identity.LoadInviteStore(inviteStorePath)
	if err != nil {
		return fmt.Errorf("loading invite store: %w", err)
	}
	n.logger.Info("invite store loaded", "path", inviteStorePath)

	return nil
}

// initTransport opens the I2P transport layer.
func (n *Node) initTransport() error {
	n.logger.Info("opening I2P transport",
		"sam_addr", n.config.I2P.SAMAddress,
		"tunnel_length", n.config.I2P.TunnelLength,
	)

	// Build I2P options from config
	options := []string{
		fmt.Sprintf("inbound.length=%d", n.config.I2P.TunnelLength),
		fmt.Sprintf("outbound.length=%d", n.config.I2P.TunnelLength),
	}

	n.trans = transport.NewTransport(n.config.Node.Name, n.config.I2P.SAMAddress, options)

	if err := n.trans.Open(); err != nil {
		return fmt.Errorf("opening I2P transport: %w", err)
	}

	// Store I2P destination in identity
	localDest := n.trans.LocalAddress()
	n.identity.SetI2PDest(localDest)

	// Save identity with I2P destination
	identityPath := filepath.Join(n.config.Node.DataDir, identity.IdentityFileName)
	if err := n.identity.Save(identityPath); err != nil {
		n.logger.Warn("failed to save identity with I2P destination", "error", err)
	}

	// Create sender for gossip message transport
	n.sender = transport.NewSender(n.trans)

	n.logger.Info("I2P transport opened",
		"local_dest", localDest[:32]+"...",
	)

	return nil
}

// initDevice creates and configures the WireGuard device.
func (n *Node) initDevice() error {
	// Parse tunnel subnet from config
	subnet, err := netip.ParsePrefix(n.config.Mesh.TunnelSubnet)
	if err != nil {
		return fmt.Errorf("parsing tunnel subnet: %w", err)
	}

	// Derive tunnel IP from our public key
	n.tunnelIP = mesh.AllocateTunnelIPWithSubnet(n.identity.PublicKey(), subnet)

	n.logger.Info("creating WireGuard device",
		"tunnel_ip", n.tunnelIP,
		"subnet", subnet,
	)

	deviceCfg := mesh.DeviceConfig{
		PrivateKey: n.identity.PrivateKey(),
		TunnelIP:   n.tunnelIP,
		Subnet:     subnet,
		MTU:        1280, // Safe for I2P
		Logger:     n.logger.With("component", "device"),
		Bind:       n.trans.Bind(),
	}

	n.device, err = mesh.NewDevice(deviceCfg)
	if err != nil {
		return fmt.Errorf("creating WireGuard device: %w", err)
	}

	n.logger.Info("WireGuard device created")
	return nil
}

// initMesh initializes mesh networking components.
func (n *Node) initMesh(ctx context.Context) error {
	// Parse subnet for routing table
	subnet, err := netip.ParsePrefix(n.config.Mesh.TunnelSubnet)
	if err != nil {
		return fmt.Errorf("parsing tunnel subnet: %w", err)
	}

	// Create routing table
	routesPath := filepath.Join(n.config.Node.DataDir, "routes.json")
	n.routing = mesh.NewRoutingTable(mesh.RoutingTableConfig{
		Subnet:   subnet,
		FilePath: routesPath,
	})

	// Add our own route
	_ = n.routing.AddRoute(&mesh.RouteEntry{
		TunnelIP:    n.tunnelIP,
		WGPublicKey: n.identity.PublicKey().String(),
		I2PDest:     n.identity.I2PDest(),
		NodeID:      n.identity.NodeID(),
		LastSeen:    time.Now(),
		CreatedAt:   time.Now(),
		HopCount:    0,
	})

	// Create ban list
	banListPath := filepath.Join(n.config.Node.DataDir, "banlist.json")
	n.banList = mesh.NewBanList(mesh.BanListConfig{
		PersistPath: banListPath,
		Logger:      n.logger.With("component", "banlist"),
	})

	// Create peer manager
	n.peers = mesh.NewPeerManager(mesh.PeerManagerConfig{
		NodeID:      n.identity.NodeID(),
		I2PDest:     n.identity.I2PDest(),
		WGPublicKey: n.identity.PublicKey(),
		TunnelIP:    n.tunnelIP,
		NetworkID:   n.identity.NetworkID(),
		MaxPeers:    n.config.Mesh.MaxPeers,
		Logger:      n.logger.With("component", "peers"),
		BanList:     n.banList,
	})

	// Wire up peer connection callbacks to register/unregister peers with the sender.
	// This is essential for gossip messaging - the sender needs to know the I2P
	// destination for each peer's nodeID before it can send messages to them.
	n.peers.SetCallbacks(
		func(peer *mesh.Peer) {
			// Peer connected - register with sender so gossip can reach them
			n.sender.RegisterPeer(peer.NodeID, peer.I2PDest)
			n.logger.Debug("registered peer with sender",
				"node_id", peer.NodeID,
				"i2p_dest", peer.I2PDest[:32]+"...")
		},
		func(peer *mesh.Peer) {
			// Peer disconnected - unregister from sender
			n.sender.UnregisterPeer(peer.NodeID)
			n.logger.Debug("unregistered peer from sender", "node_id", peer.NodeID)
		},
	)

	// Create gossip engine with defaults, then override from config
	gossipConfig := mesh.DefaultGossipConfig()
	gossipConfig.HeartbeatInterval = n.config.Mesh.HeartbeatInterval
	gossipConfig.PeerTimeout = n.config.Mesh.PeerTimeout
	gossipConfig.Logger = n.logger.With("component", "gossip")

	n.gossip = mesh.NewGossipEngine(mesh.GossipEngineConfig{
		Config:       gossipConfig,
		PeerManager:  n.peers,
		RoutingTable: n.routing,
		Sender:       n.sender,
		NodeID:       n.identity.NodeID(),
		I2PDest:      n.identity.I2PDest(),
		WGPublicKey:  n.identity.PublicKey().String(),
		TunnelIP:     n.tunnelIP.String(),
		NetworkID:    n.identity.NetworkID(),
	})

	// Start gossip engine
	if err := n.gossip.Start(ctx); err != nil {
		return fmt.Errorf("starting gossip engine: %w", err)
	}

	n.logger.Info("mesh networking initialized")
	return nil
}

// initInterfaces starts RPC and Web interfaces if enabled.
func (n *Node) initInterfaces(ctx context.Context) error {
	// Start RPC server if enabled
	if n.config.RPC.Enabled {
		socketPath := filepath.Join(n.config.Node.DataDir, n.config.RPC.Socket)
		authPath := filepath.Join(n.config.Node.DataDir, "rpc.auth")

		var err error
		n.rpcServer, err = rpc.NewServer(rpc.ServerConfig{
			AuthFile: authPath,
			Logger:   n.logger.With("component", "rpc"),
		})
		if err != nil {
			return fmt.Errorf("creating RPC server: %w", err)
		}

		// Register handlers
		handlers := rpc.NewHandlers(rpc.HandlersConfig{
			Node:   n,
			Peers:  n,
			Invite: n,
			Routes: n,
			Config: n,
			Bans:   n,
		})
		handlers.RegisterAll(n.rpcServer)

		// Start the server
		if err := n.rpcServer.Start(ctx, rpc.ServerConfig{
			UnixSocketPath: socketPath,
			TCPAddress:     n.config.RPC.TCPAddress,
		}); err != nil {
			return fmt.Errorf("starting RPC server: %w", err)
		}

		n.logger.Info("RPC server started", "socket", socketPath)
	}

	// Start Web server if enabled
	if n.config.Web.Enabled && n.config.RPC.Enabled {
		socketPath := filepath.Join(n.config.Node.DataDir, n.config.RPC.Socket)
		authPath := filepath.Join(n.config.Node.DataDir, "rpc.auth")

		var err error
		n.webServer, err = web.New(web.Config{
			ListenAddr:    n.config.Web.Listen,
			RPCSocketPath: socketPath,
			RPCAuthFile:   authPath,
			Logger:        n.logger.With("component", "web"),
		})
		if err != nil {
			return fmt.Errorf("creating web server: %w", err)
		}

		if err := n.webServer.Start(); err != nil {
			return fmt.Errorf("starting web server: %w", err)
		}

		n.logger.Info("web server started", "listen", n.config.Web.Listen)
	}

	return nil
}

// cleanup shuts down all components in reverse order.
func (n *Node) cleanup() {
	// Stop web server
	if n.webServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = n.webServer.Stop(ctx)
		cancel()
		n.webServer = nil
	}

	// Stop RPC server
	if n.rpcServer != nil {
		_ = n.rpcServer.Stop()
		n.rpcServer = nil
	}

	// Stop gossip engine
	if n.gossip != nil {
		n.gossip.Stop()
		n.gossip = nil
	}

	// Close device (may block, use timeout)
	if n.device != nil {
		done := make(chan struct{})
		go func() {
			n.device.Close()
			close(done)
		}()
		// Use two-phase timeout: warn early, then error if still blocked
		select {
		case <-done:
			n.logger.Debug("device closed successfully")
		case <-time.After(2 * time.Second):
			n.logger.Warn("device close taking longer than expected, continuing wait")
			select {
			case <-done:
				n.logger.Debug("device closed after extended wait")
			case <-time.After(3 * time.Second):
				n.logger.Error("device close timed out after 5s, continuing cleanup (resources may leak)")
			}
		}
		n.device = nil
	}

	// Close transport
	if n.trans != nil {
		_ = n.trans.Close()
		n.trans = nil
	}
}

// --- NodeProvider implementation for RPC handlers ---

// NodeName returns the configured node name.
func (n *Node) NodeName() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.Node.Name
}

// NodeID returns the unique node identifier.
func (n *Node) NodeID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.identity == nil {
		return ""
	}
	return n.identity.NodeID()
}

// TunnelIP returns our mesh tunnel IP as a string.
func (n *Node) TunnelIP() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if !n.tunnelIP.IsValid() {
		return ""
	}
	return n.tunnelIP.String()
}

// I2PDestination returns our I2P destination.
func (n *Node) I2PDestination() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.identity == nil {
		return ""
	}
	return n.identity.I2PDest()
}

// StartTime returns when the node started.
func (n *Node) StartTime() time.Time {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.startedAt
}

// Version returns the software version.
func (n *Node) Version() string {
	return version.Version
}

// PeerCount returns the number of connected peers.
func (n *Node) PeerCount() int {
	n.mu.RLock()
	pm := n.peers
	n.mu.RUnlock()

	if pm == nil {
		return 0
	}
	return pm.ConnectedCount()
}

// I2PAddress returns our short I2P address (base32.b32.i2p format).
func (n *Node) I2PAddress() string {
	n.mu.RLock()
	trans := n.trans
	n.mu.RUnlock()

	if trans == nil {
		return ""
	}
	return trans.LocalAddress()
}

// --- PeerProvider implementation for RPC handlers ---

// ListPeers returns all known peers for RPC.
func (n *Node) ListPeers() []rpc.PeerInfo {
	n.mu.RLock()
	pm := n.peers
	n.mu.RUnlock()

	if pm == nil {
		return nil
	}

	meshPeers := pm.ListPeers()
	result := make([]rpc.PeerInfo, len(meshPeers))
	for i, p := range meshPeers {
		result[i] = rpc.PeerInfo{
			NodeID:   p.NodeID,
			TunnelIP: p.TunnelIP.String(),
			State:    p.State.String(),
			LastSeen: p.LastSeen.Format(time.RFC3339),
		}
		if !p.ConnectedAt.IsZero() {
			result[i].ConnectedAt = p.ConnectedAt.Format(time.RFC3339)
		}
		if p.Latency > 0 {
			result[i].Latency = p.Latency.String()
		}
	}
	return result
}

// ConnectPeer connects to a peer using an invite code.
func (n *Node) ConnectPeer(ctx context.Context, inviteCode string) (*rpc.PeersConnectResult, error) {
	n.mu.RLock()
	pm := n.peers
	trans := n.trans
	sender := n.sender
	id := n.identity
	n.mu.RUnlock()

	if pm == nil || trans == nil || id == nil || sender == nil {
		return nil, errors.New("node not fully initialized")
	}

	// Parse the invite code
	invite, err := identity.ParseInvite(inviteCode)
	if err != nil {
		return nil, fmt.Errorf("invalid invite code: %w", err)
	}

	// Validate invite hasn't expired
	if err := invite.Validate(); err != nil {
		return nil, fmt.Errorf("invite validation failed: %w", err)
	}

	n.logger.Info("connecting to peer via invite",
		"i2p_dest", invite.I2PDest[:32]+"...",
		"network_id", invite.NetworkID)

	// Add peer to transport tracking
	if err := trans.AddPeer(invite.I2PDest, ""); err != nil {
		return nil, fmt.Errorf("failed to track peer: %w", err)
	}

	// Create handshake init with the invite's auth token
	handshakeInit := pm.CreateHandshakeInit(invite.AuthToken)

	// Parse remote endpoint
	endpoint, err := trans.ParseEndpoint(invite.I2PDest)
	if err != nil {
		trans.RemovePeer(invite.I2PDest)
		return nil, fmt.Errorf("failed to parse peer endpoint: %w", err)
	}

	// Update transport with endpoint
	trans.SetPeerEndpoint(invite.I2PDest, endpoint)

	n.logger.Info("handshake initiated",
		"our_node_id", handshakeInit.NodeID,
		"our_tunnel_ip", handshakeInit.TunnelIP)

	// Encode and send the handshake init message
	handshakeData, err := mesh.EncodeMessage(mesh.MsgHandshakeInit, handshakeInit)
	if err != nil {
		trans.RemovePeer(invite.I2PDest)
		return nil, fmt.Errorf("encoding handshake: %w", err)
	}

	// Send directly to the peer's I2P destination
	if err := sender.SendToDest(invite.I2PDest, handshakeData); err != nil {
		n.logger.Warn("failed to send handshake init",
			"error", err,
			"dest", invite.I2PDest[:32]+"...")
		// Don't fail completely - the connection may still succeed
		// as the gossip layer may retry or find an alternative route
	} else {
		n.logger.Info("handshake init sent successfully",
			"dest", invite.I2PDest[:32]+"...")
	}

	return &rpc.PeersConnectResult{
		NodeID:   "pending",
		TunnelIP: handshakeInit.TunnelIP,
		Message:  fmt.Sprintf("Handshake sent to %s...", invite.I2PDest[:32]),
	}, nil
}

// --- InviteProvider implementation for RPC handlers ---

// CreateInvite creates a new invite code for others to join the mesh.
// If this is the first node in a new network (no NetworkID set), a new
// network is automatically created with a randomly generated NetworkID.
func (n *Node) CreateInvite(expiry time.Duration, maxUses int) (*rpc.InviteCreateResult, error) {
	n.mu.RLock()
	id := n.identity
	invStore := n.inviteStore
	pm := n.peers
	n.mu.RUnlock()

	if id == nil || invStore == nil {
		return nil, errors.New("node not fully initialized")
	}

	// If no NetworkID exists, this is the first node creating a new network
	// Auto-generate a NetworkID to bootstrap the mesh
	if id.NetworkID() == "" {
		networkID, err := identity.GenerateNetworkID()
		if err != nil {
			return nil, fmt.Errorf("generating network ID: %w", err)
		}
		id.SetNetworkID(networkID)

		// Persist the identity with the new NetworkID
		identityPath := filepath.Join(n.config.Node.DataDir, identity.IdentityFileName)
		if err := id.Save(identityPath); err != nil {
			n.logger.Warn("failed to save identity with network ID", "error", err)
		}

		n.logger.Info("created new mesh network", "network_id", networkID)
	}

	// Create invite options
	opts := identity.InviteOptions{
		Expiry:  expiry,
		MaxUses: maxUses,
	}
	if opts.Expiry <= 0 {
		opts.Expiry = identity.DefaultInviteExpiry
	}
	if opts.MaxUses <= 0 {
		opts.MaxUses = identity.DefaultMaxUses
	}

	// Generate the invite
	invite, err := identity.NewInvite(id, opts)
	if err != nil {
		return nil, fmt.Errorf("generating invite: %w", err)
	}

	// Store the invite so we can validate incoming handshakes with this token
	key := invStore.AddGenerated(invite)
	if err := invStore.Save(); err != nil {
		n.logger.Warn("failed to persist invite store", "error", err)
	}

	// Add the auth token to the peer manager's valid tokens
	if pm != nil {
		pm.AddValidToken(invite.AuthToken)
	}

	// Encode the invite to a shareable code
	inviteCode, err := invite.Encode()
	if err != nil {
		return nil, fmt.Errorf("encoding invite: %w", err)
	}

	n.logger.Info("invite created",
		"key", key[:16]+"...",
		"expires_at", invite.ExpiresAt,
		"max_uses", invite.MaxUses)

	return &rpc.InviteCreateResult{
		InviteCode: inviteCode,
		ExpiresAt:  invite.ExpiresAt.Format(time.RFC3339),
		MaxUses:    invite.MaxUses,
	}, nil
}

// AcceptInvite accepts an invite code and initiates connection to the inviter.
func (n *Node) AcceptInvite(ctx context.Context, inviteCode string) (*rpc.InviteAcceptResult, error) {
	n.mu.RLock()
	id := n.identity
	invStore := n.inviteStore
	n.mu.RUnlock()

	if id == nil || invStore == nil {
		return nil, errors.New("node not fully initialized")
	}

	// Parse the invite
	invite, err := identity.ParseInvite(inviteCode)
	if err != nil {
		return nil, fmt.Errorf("invalid invite code: %w", err)
	}

	// Validate invite
	if err := invite.Validate(); err != nil {
		return nil, fmt.Errorf("invite validation failed: %w", err)
	}

	// Store as pending
	invStore.AddPending(invite)
	if err := invStore.Save(); err != nil {
		n.logger.Warn("failed to persist invite store", "error", err)
	}

	// Update our network ID to match the invite
	id.SetNetworkID(invite.NetworkID)

	// Initiate connection using ConnectPeer
	_, err = n.ConnectPeer(ctx, inviteCode)
	if err != nil {
		return nil, fmt.Errorf("connecting to peer: %w", err)
	}

	// Mark as accepted (connection initiated)
	invStore.MarkAccepted(invite.NetworkID)
	if err := invStore.Save(); err != nil {
		n.logger.Warn("failed to persist invite store", "error", err)
	}

	n.logger.Info("invite accepted",
		"network_id", invite.NetworkID,
		"peer_dest", invite.I2PDest[:32]+"...")

	return &rpc.InviteAcceptResult{
		NetworkID:  invite.NetworkID,
		PeerNodeID: invite.CreatedBy,
		TunnelIP:   n.TunnelIP(),
		Message:    "Successfully accepted invite and initiated connection",
	}, nil
}

// --- RouteProvider implementation for RPC handlers ---

// ListRoutes returns all routes for RPC.
func (n *Node) ListRoutes() []rpc.RouteInfo {
	n.mu.RLock()
	rt := n.routing
	n.mu.RUnlock()

	if rt == nil {
		return nil
	}

	routes := rt.ListRoutes()
	result := make([]rpc.RouteInfo, len(routes))
	for i, r := range routes {
		result[i] = rpc.RouteInfo{
			NodeID:    r.NodeID,
			TunnelIP:  r.TunnelIP.String(),
			HopCount:  r.HopCount,
			ViaNodeID: r.ViaNodeID,
			LastSeen:  r.LastSeen.Format(time.RFC3339),
		}
	}
	return result
}

// --- BanProvider implementation for RPC handlers ---

// ListBans returns all active bans for RPC.
func (n *Node) ListBans() []rpc.BanEntry {
	n.mu.RLock()
	bl := n.banList
	n.mu.RUnlock()

	if bl == nil {
		return nil
	}

	bans := bl.List()
	result := make([]rpc.BanEntry, len(bans))
	for i, b := range bans {
		result[i] = rpc.BanEntry{
			NodeID:      b.NodeID,
			I2PDest:     b.I2PDest,
			Reason:      string(b.Reason),
			Description: b.Description,
			BannedAt:    b.BannedAt,
			ExpiresAt:   b.ExpiresAt,
			StrikeCount: b.StrikeCount,
		}
	}
	return result
}

// AddBan adds a peer to the ban list.
func (n *Node) AddBan(nodeID, reason, description string, duration time.Duration) error {
	n.mu.RLock()
	bl := n.banList
	n.mu.RUnlock()

	if bl == nil {
		return errors.New("ban list not initialized")
	}

	bl.Ban(nodeID, mesh.BanReason(reason), description, duration)
	return nil
}

// RemoveBan removes a peer from the ban list.
func (n *Node) RemoveBan(nodeID string) bool {
	n.mu.RLock()
	bl := n.banList
	n.mu.RUnlock()

	if bl == nil {
		return false
	}

	return bl.Unban(nodeID)
}

// --- Component accessors for embedded API ---

// Identity returns the node's identity.
func (n *Node) Identity() *identity.Identity {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.identity
}

// Transport returns the I2P transport.
func (n *Node) Transport() *transport.Transport {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.trans
}

// Device returns the WireGuard device.
func (n *Node) Device() *mesh.Device {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.device
}

// RoutingTable returns the routing table.
func (n *Node) RoutingTable() *mesh.RoutingTable {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.routing
}

// PeerManager returns the peer manager.
func (n *Node) PeerManager() *mesh.PeerManager {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.peers
}

// InviteStore returns the invite store.
func (n *Node) InviteStore() *identity.InviteStore {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.inviteStore
}

// TunnelIPAddr returns the tunnel IP as netip.Addr.
func (n *Node) TunnelIPAddr() netip.Addr {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.tunnelIP
}

// --- ConfigProvider implementation for RPC handlers ---

// GetConfig returns a configuration value for the given key.
// If key is empty, returns the entire configuration.
// Supported keys: node.name, node.data_dir, i2p.sam_address, i2p.tunnel_length,
// mesh.tunnel_subnet, mesh.heartbeat_interval, mesh.peer_timeout, mesh.max_peers,
// rpc.enabled, rpc.socket, rpc.tcp_address, web.enabled, web.listen
func (n *Node) GetConfig(key string) (any, error) {
	n.mu.RLock()
	cfg := n.config
	n.mu.RUnlock()

	if cfg == nil {
		return nil, errors.New("configuration not available")
	}

	// If no key specified, return the entire config
	if key == "" {
		return cfg, nil
	}

	// Look up specific key
	switch key {
	// Node config
	case "node.name":
		return cfg.Node.Name, nil
	case "node.data_dir":
		return cfg.Node.DataDir, nil

	// I2P config
	case "i2p.sam_address":
		return cfg.I2P.SAMAddress, nil
	case "i2p.tunnel_length":
		return cfg.I2P.TunnelLength, nil

	// Mesh config
	case "mesh.tunnel_subnet":
		return cfg.Mesh.TunnelSubnet, nil
	case "mesh.heartbeat_interval":
		return cfg.Mesh.HeartbeatInterval.String(), nil
	case "mesh.peer_timeout":
		return cfg.Mesh.PeerTimeout.String(), nil
	case "mesh.max_peers":
		return cfg.Mesh.MaxPeers, nil

	// RPC config
	case "rpc.enabled":
		return cfg.RPC.Enabled, nil
	case "rpc.socket":
		return cfg.RPC.Socket, nil
	case "rpc.tcp_address":
		return cfg.RPC.TCPAddress, nil

	// Web config
	case "web.enabled":
		return cfg.Web.Enabled, nil
	case "web.listen":
		return cfg.Web.Listen, nil

	default:
		return nil, fmt.Errorf("unknown config key: %s", key)
	}
}

// SetConfig sets a configuration value.
// Note: Most configuration changes require a restart to take effect.
// Returns the old value if successful.
func (n *Node) SetConfig(key string, value any) (any, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.config == nil {
		return nil, errors.New("configuration not available")
	}

	if key == "" {
		return nil, errors.New("key is required")
	}

	var oldValue any

	switch key {
	// Node config
	case "node.name":
		oldValue = n.config.Node.Name
		if s, ok := value.(string); ok {
			n.config.Node.Name = s
		} else {
			return nil, errors.New("node.name must be a string")
		}

	// Mesh config (runtime adjustable)
	case "mesh.max_peers":
		oldValue = n.config.Mesh.MaxPeers
		switch v := value.(type) {
		case int:
			n.config.Mesh.MaxPeers = v
		case float64:
			n.config.Mesh.MaxPeers = int(v)
		default:
			return nil, errors.New("mesh.max_peers must be an integer")
		}

	// RPC config
	case "rpc.tcp_address":
		oldValue = n.config.RPC.TCPAddress
		if s, ok := value.(string); ok {
			n.config.RPC.TCPAddress = s
		} else {
			return nil, errors.New("rpc.tcp_address must be a string")
		}

	// Read-only configs
	case "node.data_dir", "i2p.sam_address", "i2p.tunnel_length",
		"mesh.tunnel_subnet", "mesh.heartbeat_interval", "mesh.peer_timeout",
		"rpc.enabled", "rpc.socket", "web.enabled", "web.listen":
		return nil, fmt.Errorf("config key %s is read-only at runtime", key)

	default:
		return nil, fmt.Errorf("unknown config key: %s", key)
	}

	n.logger.Info("configuration updated", "key", key, "old_value", oldValue, "new_value", value)
	return oldValue, nil
}
