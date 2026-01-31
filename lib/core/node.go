package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-i2p/i2pkeys"
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

	// Resilience components
	stateManager     *mesh.StateManager
	reconnectManager *mesh.ReconnectManager
	healthMonitor    *transport.HealthMonitor

	// User interfaces
	rpcServer *rpc.Server
	webServer *web.Server

	// Derived values
	tunnelIP netip.Addr
}

// NewNode creates a new Node with the given configuration.
// The node is not started until Start() is called.
func NewNode(cfg *Config) (*Node, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Node{
		config: cfg,
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
	if err := n.transitionToStarting(); err != nil {
		return err
	}

	nodeCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	n.logStarting()

	if err := n.initializeAllComponents(nodeCtx); err != nil {
		return err
	}

	n.transitionToRunning()
	go n.run(nodeCtx)

	return nil
}

// transitionToStarting attempts to transition the node to the starting state.
func (n *Node) transitionToStarting() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state != StateInitial && n.state != StateStopped {
		return fmt.Errorf("cannot start node in state %s", n.state)
	}
	oldState := n.state
	n.state = StateStarting
	n.done = make(chan struct{})

	// Must emit outside of lock in real implementation
	go n.emitStateChange(oldState, StateStarting)
	return nil
}

// logStarting logs the node startup information.
func (n *Node) logStarting() {
	log.Info("starting node",
		"name", n.config.Node.Name,
		"data_dir", n.config.Node.DataDir,
	)
}

// initializeAllComponents initializes all node components in order.
func (n *Node) initializeAllComponents(nodeCtx context.Context) error {
	if err := n.initDataDir(); err != nil {
		return err
	}

	if err := n.initIdentityPhase(); err != nil {
		return err
	}

	if err := n.initTransportPhase(); err != nil {
		return err
	}

	if err := n.initMeshPhase(nodeCtx); err != nil {
		return err
	}

	return n.initInterfacesPhase(nodeCtx)
}

// initDataDir ensures the data directory exists.
func (n *Node) initDataDir() error {
	if err := n.config.EnsureDataDir(); err != nil {
		n.transitionToStopped()
		n.emitError(err, "failed to create data directory")
		return fmt.Errorf("creating data directory: %w", err)
	}
	return nil
}

// initIdentityPhase loads or generates identity.
func (n *Node) initIdentityPhase() error {
	if err := n.initIdentity(); err != nil {
		n.transitionToStopped()
		n.emitError(err, "failed to initialize identity")
		return err
	}
	return nil
}

// initTransportPhase opens I2P transport and WireGuard device.
func (n *Node) initTransportPhase() error {
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
	return nil
}

// initMeshPhase initializes mesh components and starts gossip.
func (n *Node) initMeshPhase(nodeCtx context.Context) error {
	if err := n.initMesh(nodeCtx); err != nil {
		n.cleanup()
		n.transitionToStopped()
		n.emitError(err, "failed to initialize mesh")
		return err
	}
	return nil
}

// initInterfacesPhase starts user interfaces if enabled.
func (n *Node) initInterfacesPhase(nodeCtx context.Context) error {
	if err := n.initInterfaces(nodeCtx); err != nil {
		n.cleanup()
		n.transitionToStopped()
		n.emitError(err, "failed to initialize interfaces")
		return err
	}
	return nil
}

// transitionToRunning sets the node state to running.
func (n *Node) transitionToRunning() {
	n.mu.Lock()
	n.state = StateRunning
	n.startedAt = time.Now()
	n.mu.Unlock()

	n.emitStateChange(StateStarting, StateRunning)
	log.Info("node started")
}

// run is the main loop that runs until the context is cancelled.
func (n *Node) run(ctx context.Context) {
	defer close(n.done)

	<-ctx.Done()

	log.Info("node shutting down")

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
	log.Info("stopping node")

	// Signal all goroutines to stop
	if cancel != nil {
		cancel()
	}

	// Wait for the run loop to finish or timeout
	select {
	case <-n.done:
		log.Info("node stopped")
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

	log.Info("loading identity", "path", identityPath)

	id, err := identity.LoadIdentity(identityPath)
	if err != nil {
		return fmt.Errorf("loading identity: %w", err)
	}

	if id == nil {
		// No existing identity, generate a new one
		log.Info("generating new identity")
		id, err = identity.NewIdentity()
		if err != nil {
			return fmt.Errorf("generating identity: %w", err)
		}

		if err := id.Save(identityPath); err != nil {
			return fmt.Errorf("saving identity: %w", err)
		}
	}

	n.identity = id
	log.Info("identity loaded",
		"node_id", id.NodeID(),
		"public_key", id.PublicKey().String()[:16]+"...",
	)

	// Load or create invite store
	inviteStorePath := filepath.Join(n.config.Node.DataDir, "invites.json")
	n.inviteStore, err = identity.LoadInviteStore(inviteStorePath)
	if err != nil {
		return fmt.Errorf("loading invite store: %w", err)
	}
	log.Info("invite store loaded", "path", inviteStorePath)

	return nil
}

// initTransport opens the I2P transport layer.
func (n *Node) initTransport() error {
	log.Info("opening I2P transport",
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
	// Use full base64-encoded destination for invite compatibility
	localDest, err := n.trans.LocalDestination()
	if err != nil {
		return fmt.Errorf("getting local I2P destination: %w", err)
	}
	n.identity.SetI2PDest(string(localDest))

	// Save identity with I2P destination
	identityPath := filepath.Join(n.config.Node.DataDir, identity.IdentityFileName)
	if err := n.identity.Save(identityPath); err != nil {
		log.Warn("failed to save identity with I2P destination", "error", err)
	}

	// Create sender for gossip message transport
	n.sender = transport.NewSender(n.trans)

	// Create health monitor for SAM connection
	n.healthMonitor = transport.NewHealthMonitor(n.config.I2P.SAMAddress, transport.HealthConfig{
		CheckInterval: 30 * time.Second,
		Timeout:       5 * time.Second,
		RetryDelay:    5 * time.Second,
		MaxRetryDelay: 2 * time.Minute,
	})

	// Log with base32 address for readability
	base32Addr := localDest.Base32()
	log.Info("I2P transport opened",
		"local_dest", base32Addr[:32]+"...",
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

	log.Info("creating WireGuard device",
		"tunnel_ip", n.tunnelIP,
		"subnet", subnet,
	)

	deviceCfg := mesh.DeviceConfig{
		PrivateKey: n.identity.PrivateKey(),
		TunnelIP:   n.tunnelIP,
		Subnet:     subnet,
		MTU:        1280, // Safe for I2P
		Bind:       n.trans.Bind(),
	}

	n.device, err = mesh.NewDevice(deviceCfg)
	if err != nil {
		return fmt.Errorf("creating WireGuard device: %w", err)
	}

	log.Info("WireGuard device created")
	return nil
}

// initMesh initializes mesh networking components.
func (n *Node) initMesh(ctx context.Context) error {
	// Initialize routing table
	if err := n.initRoutingTable(); err != nil {
		return err
	}

	// Initialize ban list
	n.initBanList()

	// Initialize peer manager
	n.initPeerManager()

	// Initialize state and reconnect managers
	n.initStateManagement()

	// Configure peer callbacks for sender and WireGuard device
	n.configurePeerCallbacks()

	// Initialize gossip engine
	if err := n.initGossipEngine(ctx); err != nil {
		return err
	}

	// Start background services
	if err := n.startMeshServices(ctx); err != nil {
		return err
	}

	log.Info("mesh networking initialized")
	return nil
}

// initRoutingTable creates and initializes the routing table with persisted routes.
func (n *Node) initRoutingTable() error {
	subnet, err := netip.ParsePrefix(n.config.Mesh.TunnelSubnet)
	if err != nil {
		return fmt.Errorf("parsing tunnel subnet: %w", err)
	}

	routesPath := filepath.Join(n.config.Node.DataDir, "routes.json")
	n.routing = mesh.NewRoutingTable(mesh.RoutingTableConfig{
		Subnet:   subnet,
		FilePath: routesPath,
	})

	if err := n.routing.Load(); err != nil {
		log.Debug("no persisted routes to load", "error", err)
	} else {
		log.Info("loaded persisted routes", "count", n.routing.RouteCount())
	}

	_ = n.routing.AddRoute(&mesh.RouteEntry{
		TunnelIP:    n.tunnelIP,
		WGPublicKey: n.identity.PublicKey().String(),
		I2PDest:     n.identity.I2PDest(),
		NodeID:      n.identity.NodeID(),
		LastSeen:    time.Now(),
		CreatedAt:   time.Now(),
		HopCount:    0,
	})

	return nil
}

// initBanList creates and configures the ban list for peer management.
func (n *Node) initBanList() {
	banListPath := filepath.Join(n.config.Node.DataDir, "banlist.json")
	n.banList = mesh.NewBanList(mesh.BanListConfig{
		PersistPath: banListPath,
	})
}

// initPeerManager creates and configures the peer manager.
func (n *Node) initPeerManager() {
	n.peers = mesh.NewPeerManager(mesh.PeerManagerConfig{
		NodeID:       n.identity.NodeID(),
		I2PDest:      n.identity.I2PDest(),
		WGPublicKey:  n.identity.PublicKey(),
		TunnelIP:     n.tunnelIP,
		NetworkID:    n.identity.NetworkID(),
		MaxPeers:     n.config.Mesh.MaxPeers,
		BanList:      n.banList,
		RoutingTable: n.routing,
		Subnet:       n.routing.Subnet(),
	})
}

// initStateManagement creates state manager and reconnect manager for persistence.
func (n *Node) initStateManagement() {
	statePath := filepath.Join(n.config.Node.DataDir, mesh.StateFileName)
	n.stateManager = mesh.NewStateManager(mesh.StateManagerConfig{
		Path:         statePath,
		SaveInterval: 5 * time.Minute,
		PeerManager:  n.peers,
		RoutingTable: n.routing,
	})

	if err := n.stateManager.Load(); err != nil {
		log.Debug("no persisted state to load", "error", err)
	} else {
		log.Info("loaded persisted state")
	}

	n.reconnectManager = mesh.NewReconnectManager(mesh.ReconnectConfig{
		InitialDelay:   5 * time.Second,
		MaxDelay:       5 * time.Minute,
		Multiplier:     2.0,
		MaxRetries:     0,
		JitterFraction: 0.2,
		CheckInterval:  10 * time.Second,
	})
}

// configurePeerCallbacks sets up callbacks for peer connection and disconnection events.
func (n *Node) configurePeerCallbacks() {
	n.peers.SetCallbacks(
		func(peer *mesh.Peer) { n.handlePeerConnected(peer) },
		func(peer *mesh.Peer) { n.handlePeerDisconnected(peer) },
	)

	n.peers.SetTokenUsedCallback(func(token []byte) {
		n.handleTokenUsed(token)
	})
}

// handlePeerConnected registers a peer with the sender and WireGuard device.
func (n *Node) handlePeerConnected(peer *mesh.Peer) {
	n.sender.RegisterPeer(peer.NodeID, peer.I2PDest)
	log.Debug("registered peer with sender",
		"node_id", peer.NodeID,
		"i2p_dest", peer.I2PDest[:32]+"...")

	allowedIP := netip.PrefixFrom(peer.TunnelIP, 32)
	if err := n.device.AddPeer(peer.WGPublicKey, []netip.Prefix{allowedIP}, peer.I2PDest); err != nil {
		log.Error("failed to add WireGuard peer",
			"node_id", peer.NodeID,
			"error", err)
	} else {
		log.Info("added WireGuard peer",
			"node_id", peer.NodeID,
			"tunnel_ip", peer.TunnelIP,
			"wg_pubkey", peer.WGPublicKey.String()[:8]+"...")
	}
}

// handlePeerDisconnected unregisters a peer and queues reconnection.
func (n *Node) handlePeerDisconnected(peer *mesh.Peer) {
	n.sender.UnregisterPeer(peer.NodeID)
	log.Debug("unregistered peer from sender", "node_id", peer.NodeID)

	if err := n.device.RemovePeer(peer.WGPublicKey); err != nil {
		log.Error("failed to remove WireGuard peer",
			"node_id", peer.NodeID,
			"error", err)
	} else {
		log.Debug("removed WireGuard peer", "node_id", peer.NodeID)
	}

	if n.reconnectManager != nil {
		n.reconnectManager.Add(peer.NodeID, peer.I2PDest, nil)
		log.Debug("queued peer for reconnection", "node_id", peer.NodeID)
	}
}

// initGossipEngine creates and configures the gossip engine for mesh networking.
func (n *Node) initGossipEngine(ctx context.Context) error {
	gossipConfig := mesh.DefaultGossipConfig()
	gossipConfig.HeartbeatInterval = n.config.Mesh.HeartbeatInterval
	gossipConfig.PeerTimeout = n.config.Mesh.PeerTimeout

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

	n.trans.SetMeshHandler(func(data []byte, from i2pkeys.I2PAddr) {
		n.handleMeshMessage(data, from)
	})

	n.gossip.SetDiscoveryCallback(func(peerInfo mesh.PeerInfo) {
		go n.connectToDiscoveredPeer(peerInfo)
	})

	if networkID := n.identity.NetworkID(); networkID != "" {
		discoveryToken := identity.DeriveDiscoveryToken(networkID)
		n.peers.AddValidToken(discoveryToken)
		log.Debug("added network discovery token for auto-connect")
	}

	return nil
}

// handleMeshMessage parses and routes incoming mesh protocol messages.
func (n *Node) handleMeshMessage(data []byte, from i2pkeys.I2PAddr) {
	var msg mesh.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Debug("failed to parse mesh message", "error", err, "from", from.Base32()[:16]+"...")
		return
	}
	if err := n.gossip.HandleMessage(&msg); err != nil {
		log.Debug("failed to handle mesh message",
			"type", msg.Type,
			"error", err,
			"from", from.Base32()[:16]+"...")
	}
}

// startMeshServices starts all mesh-related background services.
func (n *Node) startMeshServices(ctx context.Context) error {
	if err := n.gossip.Start(ctx); err != nil {
		return fmt.Errorf("starting gossip engine: %w", err)
	}

	if err := n.banList.Start(ctx); err != nil {
		return fmt.Errorf("starting ban list cleanup: %w", err)
	}

	n.configureReconnectManager(ctx)
	n.startStateManager()
	n.startInviteCleanup(ctx)
	n.startHealthMonitor(ctx)

	return nil
}

// configureReconnectManager sets up the reconnection handler and starts the manager.
func (n *Node) configureReconnectManager(ctx context.Context) {
	if n.reconnectManager == nil {
		return
	}

	n.reconnectManager.SetReconnectHandler(func(nodeID, i2pDest string, authToken []byte) error {
		if authToken == nil {
			if networkID := n.identity.NetworkID(); networkID != "" {
				authToken = identity.DeriveDiscoveryToken(networkID)
			}
		}
		return n.attemptPeerReconnect(nodeID, i2pDest, authToken)
	})

	if err := n.reconnectManager.Start(ctx); err != nil {
		log.Warn("failed to start reconnect manager", "error", err)
	}
}

// startStateManager starts the state manager for periodic state saving.
func (n *Node) startStateManager() {
	if n.stateManager != nil {
		n.stateManager.Start()
	}
}

// startInviteCleanup starts a background goroutine to clean expired invites.
func (n *Node) startInviteCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if removed := n.inviteStore.CleanExpired(); removed > 0 {
					log.Info("cleaned expired invites", "count", removed)
				}
			}
		}
	}()
}

// startHealthMonitor configures and starts the SAM connection health monitor.
func (n *Node) startHealthMonitor(ctx context.Context) {
	if n.healthMonitor == nil {
		return
	}

	n.healthMonitor.SetCallbacks(
		func() { log.Warn("I2P SAM connection unhealthy") },
		func() { log.Info("I2P SAM connection healthy") },
		n.handleI2PReconnect,
	)

	if err := n.healthMonitor.Start(ctx); err != nil {
		log.Warn("failed to start health monitor", "error", err)
	}
}

// handleI2PReconnect attempts to reconnect the I2P transport when SAM becomes available.
// The I2P identity is persistent (keys stored by tunnel name), so the same destination
// is restored after reconnection.
func (n *Node) handleI2PReconnect() error {
	log.Info("attempting I2P transport reconnection")

	// Attempt to reconnect the transport
	dest, err := n.trans.Reconnect()
	if err != nil {
		log.Error("I2P transport reconnection failed", "error", err)
		return err
	}

	log.Info("I2P transport reconnected", "dest", dest[:32]+"...")

	// Re-register mesh handler with reconnected transport
	n.trans.SetMeshHandler(n.handleMeshMessage)

	// Re-announce our presence to the network
	if n.gossip != nil {
		n.gossip.AnnouncePresence()
		log.Info("re-announced presence to mesh network after reconnect")
	}

	return nil
}

// initInterfaces starts RPC and Web interfaces if enabled.
func (n *Node) initInterfaces(ctx context.Context) error {
	if n.config.RPC.Enabled {
		if err := n.initRPCServer(ctx); err != nil {
			return err
		}
	}

	if n.config.Web.Enabled && n.config.RPC.Enabled {
		if err := n.initWebServer(); err != nil {
			return err
		}
	}

	return nil
}

// initRPCServer creates and starts the RPC server.
func (n *Node) initRPCServer(ctx context.Context) error {
	socketPath := filepath.Join(n.config.Node.DataDir, n.config.RPC.Socket)
	authPath := filepath.Join(n.config.Node.DataDir, "rpc.auth")

	var err error
	n.rpcServer, err = rpc.NewServer(rpc.ServerConfig{
		AuthFile: authPath,
	})
	if err != nil {
		return fmt.Errorf("creating RPC server: %w", err)
	}

	handlers := rpc.NewHandlers(rpc.HandlersConfig{
		Node:   n,
		Peers:  n,
		Invite: n,
		Routes: n,
		Config: n,
		Bans:   n,
	})
	handlers.RegisterAll(n.rpcServer)

	if err := n.rpcServer.Start(ctx, rpc.ServerConfig{
		UnixSocketPath: socketPath,
		TCPAddress:     n.config.RPC.TCPAddress,
	}); err != nil {
		return fmt.Errorf("starting RPC server: %w", err)
	}

	log.Info("RPC server started", "socket", socketPath)
	return nil
}

// initWebServer creates and starts the Web server.
func (n *Node) initWebServer() error {
	socketPath := filepath.Join(n.config.Node.DataDir, n.config.RPC.Socket)
	authPath := filepath.Join(n.config.Node.DataDir, "rpc.auth")

	var err error
	n.webServer, err = web.New(web.Config{
		ListenAddr:    n.config.Web.Listen,
		RPCSocketPath: socketPath,
		RPCAuthFile:   authPath,
	})
	if err != nil {
		return fmt.Errorf("creating web server: %w", err)
	}

	if err := n.webServer.Start(); err != nil {
		return fmt.Errorf("starting web server: %w", err)
	}

	log.Info("web server started", "listen", n.config.Web.Listen)
	return nil
}

// cleanup shuts down all components in reverse order with progress logging.
func (n *Node) cleanup() {
	log.Info("cleanup phase 1/5: stopping interfaces")
	n.cleanupInterfaces()
	log.Info("cleanup phase 2/5: stopping mesh services")
	n.cleanupMeshServices()
	log.Info("cleanup phase 3/5: persisting state")
	n.cleanupPersistence()
	log.Info("cleanup phase 4/5: closing device")
	n.cleanupDevice()
	log.Info("cleanup phase 5/5: closing transport")
	n.cleanupTransport()
	log.Info("cleanup complete")
}

// getDrainTimeout returns the configured drain timeout or default.
func (n *Node) getDrainTimeout() time.Duration {
	timeout := n.config.Mesh.DrainTimeout
	if timeout < time.Second {
		return DefaultDrainTimeout
	}
	return timeout
}

// cleanupInterfaces stops web and RPC servers with request draining.
func (n *Node) cleanupInterfaces() {
	drainTimeout := n.getDrainTimeout()

	if n.webServer != nil {
		log.Debug("stopping web server, draining requests", "timeout", drainTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
		if err := n.webServer.Stop(ctx); err != nil {
			log.Warn("web server shutdown interrupted", "error", err)
		}
		cancel()
		n.webServer = nil
	}

	if n.rpcServer != nil {
		log.Debug("stopping RPC server, draining requests", "timeout", drainTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
		if err := n.rpcServer.StopWithContext(ctx); err != nil {
			log.Warn("RPC server shutdown interrupted", "error", err)
		}
		cancel()
		n.rpcServer = nil
	}
}

// cleanupMeshServices stops gossip, reconnect, and ban list services.
func (n *Node) cleanupMeshServices() {
	if n.gossip != nil {
		log.Debug("sending leave announcement to peers")
		n.gossip.AnnounceLeave("node shutting down")
		log.Debug("stopping gossip engine")
		n.gossip.Stop()
		n.gossip = nil
	}

	if n.reconnectManager != nil {
		log.Debug("stopping reconnect manager")
		n.reconnectManager.Stop()
		n.reconnectManager = nil
	}

	if n.banList != nil {
		log.Debug("stopping ban list manager")
		n.banList.Stop()
	}
}

// cleanupPersistence saves state and routing table before shutdown.
func (n *Node) cleanupPersistence() {
	if n.stateManager != nil {
		if err := n.stateManager.Save(); err != nil {
			log.Warn("failed to save state", "error", err)
		} else {
			log.Debug("saved state before shutdown")
		}
		n.stateManager.Stop()
		n.stateManager = nil
	}

	if n.routing != nil {
		if err := n.routing.Save(); err != nil {
			log.Warn("failed to save routing table", "error", err)
		} else {
			log.Debug("saved routing table", "count", n.routing.RouteCount())
		}
	}
}

// cleanupDevice closes the WireGuard device with a two-phase timeout.
// getShutdownTimeout returns the configured shutdown timeout or default.
func (n *Node) getShutdownTimeout() time.Duration {
	timeout := n.config.Mesh.ShutdownTimeout
	if timeout < time.Second {
		return DefaultShutdownTimeout
	}
	return timeout
}

// waitForDeviceClose waits for device close with timeout and logging.
func (n *Node) waitForDeviceClose(done <-chan struct{}, shutdownTimeout time.Duration) {
	warnTimeout := shutdownTimeout * 2 / 5
	remainingTimeout := shutdownTimeout - warnTimeout

	select {
	case <-done:
		log.Debug("device closed successfully")
	case <-time.After(warnTimeout):
		log.Warn("device close taking longer than expected, continuing wait")
		select {
		case <-done:
			log.Debug("device closed after extended wait")
		case <-time.After(remainingTimeout):
			log.Error("device close timed out after configured timeout, continuing cleanup (resources may leak)",
				"timeout", shutdownTimeout.String())
		}
	}
}

func (n *Node) cleanupDevice() {
	if n.device == nil {
		return
	}

	// Capture device reference before starting goroutine to avoid race condition.
	// The goroutine may still be running when we set n.device = nil below.
	dev := n.device
	shutdownTimeout := n.getShutdownTimeout()

	done := make(chan struct{})
	go func() {
		dev.Close()
		close(done)
	}()

	n.waitForDeviceClose(done, shutdownTimeout)
	n.device = nil
}

// cleanupTransport closes the I2P transport and health monitor.
func (n *Node) cleanupTransport() {
	if n.trans != nil {
		_ = n.trans.Close()
		n.trans = nil
	}

	if n.healthMonitor != nil {
		n.healthMonitor.Stop()
		n.healthMonitor = nil
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
	pm, trans, sender, id := n.getConnectionComponents()
	if err := n.validateConnectPeerComponents(pm, trans, sender, id); err != nil {
		return nil, err
	}

	invite, err := n.parseAndValidateInvite(inviteCode)
	if err != nil {
		return nil, err
	}

	handshakeInit, err := n.setupPeerConnection(pm, trans, invite)
	if err != nil {
		return nil, err
	}

	n.sendConnectPeerHandshake(sender, trans, invite, handshakeInit)

	return &rpc.PeersConnectResult{
		NodeID:   "pending",
		TunnelIP: handshakeInit.TunnelIP,
		Message:  fmt.Sprintf("Handshake sent to %s...", invite.I2PDest[:32]),
	}, nil
}

// validateConnectPeerComponents checks if all required components are initialized.
func (n *Node) validateConnectPeerComponents(pm *mesh.PeerManager, trans *transport.Transport, sender *transport.Sender, id *identity.Identity) error {
	if pm == nil || trans == nil || id == nil || sender == nil {
		return errors.New("node not fully initialized")
	}
	return nil
}

// parseAndValidateInvite parses and validates an invite code.
func (n *Node) parseAndValidateInvite(inviteCode string) (*identity.Invite, error) {
	invite, err := identity.ParseInvite(inviteCode)
	if err != nil {
		return nil, fmt.Errorf("invalid invite code: %w", err)
	}

	if err := invite.Validate(); err != nil {
		return nil, fmt.Errorf("invite validation failed: %w", err)
	}

	log.Info("connecting to peer via invite",
		"i2p_dest", invite.I2PDest[:32]+"...",
		"network_id", invite.NetworkID)

	return invite, nil
}

// setupPeerConnection sets up transport tracking and creates handshake init.
func (n *Node) setupPeerConnection(pm *mesh.PeerManager, trans *transport.Transport, invite *identity.Invite) (*mesh.HandshakeInit, error) {
	if err := trans.AddPeer(invite.I2PDest, ""); err != nil {
		return nil, fmt.Errorf("failed to track peer: %w", err)
	}

	handshakeInit := pm.CreateHandshakeInit(invite.AuthToken)

	endpoint, err := trans.ParseEndpoint(invite.I2PDest)
	if err != nil {
		trans.RemovePeer(invite.I2PDest)
		return nil, fmt.Errorf("failed to parse peer endpoint: %w", err)
	}

	trans.SetPeerEndpoint(invite.I2PDest, endpoint)

	log.Info("handshake initiated",
		"our_node_id", handshakeInit.NodeID,
		"our_tunnel_ip", handshakeInit.TunnelIP)

	return handshakeInit, nil
}

// sendConnectPeerHandshake encodes and sends the handshake init message.
func (n *Node) sendConnectPeerHandshake(sender *transport.Sender, trans *transport.Transport, invite *identity.Invite, handshakeInit *mesh.HandshakeInit) {
	handshakeData, err := mesh.EncodeMessage(mesh.MsgHandshakeInit, handshakeInit)
	if err != nil {
		trans.RemovePeer(invite.I2PDest)
		log.Warn("failed to encode handshake", "error", err)
		return
	}

	if err := sender.SendToDest(invite.I2PDest, handshakeData); err != nil {
		log.Warn("failed to send handshake init",
			"error", err,
			"dest", invite.I2PDest[:32]+"...")
	} else {
		log.Info("handshake init sent successfully",
			"dest", invite.I2PDest[:32]+"...")
	}
}

// connectToDiscoveredPeer attempts to connect to a peer discovered via gossip.
// Uses the network's discovery token for authentication.
func (n *Node) connectToDiscoveredPeer(peerInfo mesh.PeerInfo) {
	pm, trans, sender, id := n.getConnectionComponents()
	if !n.validateConnectionComponents(pm, trans, sender, id) {
		return
	}

	if n.isPeerAlreadyConnectedOrPending(pm, peerInfo.NodeID) {
		return
	}

	n.initiateDiscoveredPeerHandshake(pm, trans, sender, id, peerInfo)
}

// getConnectionComponents retrieves the components needed for peer connection.
func (n *Node) getConnectionComponents() (*mesh.PeerManager, *transport.Transport, *transport.Sender, *identity.Identity) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.peers, n.trans, n.sender, n.identity
}

// validateConnectionComponents checks if all required components are initialized.
func (n *Node) validateConnectionComponents(pm *mesh.PeerManager, trans *transport.Transport, sender *transport.Sender, id *identity.Identity) bool {
	if pm == nil || trans == nil || id == nil || sender == nil {
		log.Warn("cannot connect to discovered peer: node not fully initialized")
		return false
	}
	return true
}

// isPeerAlreadyConnectedOrPending checks if a peer is already connected or pending connection.
func (n *Node) isPeerAlreadyConnectedOrPending(pm *mesh.PeerManager, nodeID string) bool {
	if peer, ok := pm.GetPeer(nodeID); ok {
		if peer.State == mesh.PeerStateConnected || peer.State == mesh.PeerStatePending {
			log.Debug("peer already connected or pending",
				"node_id", nodeID,
				"state", peer.State)
			return true
		}
	}
	return false
}

// initiateDiscoveredPeerHandshake performs the handshake sequence with a discovered peer.
func (n *Node) initiateDiscoveredPeerHandshake(pm *mesh.PeerManager, trans *transport.Transport, sender *transport.Sender, id *identity.Identity, peerInfo mesh.PeerInfo) {
	log.Info("auto-connecting to discovered peer",
		"node_id", peerInfo.NodeID,
		"i2p_dest", truncateDest(peerInfo.I2PDest))

	if err := trans.AddPeer(peerInfo.I2PDest, peerInfo.NodeID); err != nil {
		log.Warn("failed to track discovered peer", "error", err)
		return
	}

	discoveryToken := identity.DeriveDiscoveryToken(id.NetworkID())
	handshakeInit := pm.CreateHandshakeInit(discoveryToken)

	endpoint, err := trans.ParseEndpoint(peerInfo.I2PDest)
	if err != nil {
		trans.RemovePeer(peerInfo.I2PDest)
		log.Warn("failed to parse discovered peer endpoint", "error", err)
		return
	}

	trans.SetPeerEndpoint(peerInfo.I2PDest, endpoint)

	n.sendDiscoveredPeerHandshake(sender, trans, peerInfo, handshakeInit)
}

// sendDiscoveredPeerHandshake encodes and sends a handshake to a discovered peer.
func (n *Node) sendDiscoveredPeerHandshake(sender *transport.Sender, trans *transport.Transport, peerInfo mesh.PeerInfo, handshakeInit *mesh.HandshakeInit) {
	handshakeData, err := mesh.EncodeMessage(mesh.MsgHandshakeInit, handshakeInit)
	if err != nil {
		trans.RemovePeer(peerInfo.I2PDest)
		log.Warn("failed to encode handshake for discovered peer", "error", err)
		return
	}

	if err := sender.SendToDest(peerInfo.I2PDest, handshakeData); err != nil {
		log.Warn("failed to send handshake to discovered peer",
			"error", err,
			"node_id", peerInfo.NodeID)
	} else {
		log.Info("handshake sent to discovered peer",
			"node_id", peerInfo.NodeID)
	}
}

// attemptPeerReconnect attempts to reconnect to a previously connected peer.
// This is called by the reconnect manager when a peer is due for retry.
func (n *Node) attemptPeerReconnect(nodeID, i2pDest string, authToken []byte) error {
	pm, trans, sender := n.getReconnectComponents()
	if err := n.validateReconnectComponents(pm, trans, sender); err != nil {
		return err
	}

	if n.isPeerAlreadyReconnected(pm, nodeID) {
		return nil
	}

	return n.performReconnectHandshake(pm, trans, sender, nodeID, i2pDest, authToken)
}

// getReconnectComponents retrieves the components needed for peer reconnection.
func (n *Node) getReconnectComponents() (*mesh.PeerManager, *transport.Transport, *transport.Sender) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.peers, n.trans, n.sender
}

// validateReconnectComponents checks if all required components are initialized for reconnection.
func (n *Node) validateReconnectComponents(pm *mesh.PeerManager, trans *transport.Transport, sender *transport.Sender) error {
	if pm == nil || trans == nil || sender == nil {
		return fmt.Errorf("node not fully initialized")
	}
	return nil
}

// isPeerAlreadyReconnected checks if a peer is already connected.
func (n *Node) isPeerAlreadyReconnected(pm *mesh.PeerManager, nodeID string) bool {
	if peer, ok := pm.GetPeer(nodeID); ok {
		if peer.State == mesh.PeerStateConnected {
			log.Debug("peer already reconnected", "node_id", nodeID)
			return true
		}
	}
	return false
}

// performReconnectHandshake executes the reconnection handshake sequence.
func (n *Node) performReconnectHandshake(pm *mesh.PeerManager, trans *transport.Transport, sender *transport.Sender, nodeID, i2pDest string, authToken []byte) error {
	log.Info("attempting peer reconnection",
		"node_id", nodeID,
		"i2p_dest", truncateDest(i2pDest))

	if err := trans.AddPeer(i2pDest, nodeID); err != nil {
		return fmt.Errorf("failed to track peer: %w", err)
	}

	handshakeInit := pm.CreateHandshakeInit(authToken)

	endpoint, err := trans.ParseEndpoint(i2pDest)
	if err != nil {
		trans.RemovePeer(i2pDest)
		return fmt.Errorf("failed to parse endpoint: %w", err)
	}

	trans.SetPeerEndpoint(i2pDest, endpoint)

	return n.sendReconnectHandshake(sender, trans, nodeID, i2pDest, handshakeInit)
}

// sendReconnectHandshake encodes and sends a handshake for reconnection.
func (n *Node) sendReconnectHandshake(sender *transport.Sender, trans *transport.Transport, nodeID, i2pDest string, handshakeInit *mesh.HandshakeInit) error {
	handshakeData, err := mesh.EncodeMessage(mesh.MsgHandshakeInit, handshakeInit)
	if err != nil {
		trans.RemovePeer(i2pDest)
		return fmt.Errorf("failed to encode handshake: %w", err)
	}

	if err := sender.SendToDest(i2pDest, handshakeData); err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	log.Info("reconnection handshake sent", "node_id", nodeID)
	return nil
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

	if err := n.ensureNetworkID(id, pm); err != nil {
		return nil, err
	}

	invite, err := n.generateInvite(id, expiry, maxUses)
	if err != nil {
		return nil, err
	}

	key := n.storeAndRegisterInvite(invStore, pm, invite)

	return n.buildInviteResult(invite, key)
}

// ensureNetworkID creates a new network ID if one doesn't exist.
func (n *Node) ensureNetworkID(id *identity.Identity, pm *mesh.PeerManager) error {
	if id.NetworkID() != "" {
		return nil
	}

	networkID, err := identity.GenerateNetworkID()
	if err != nil {
		return fmt.Errorf("generating network ID: %w", err)
	}
	id.SetNetworkID(networkID)

	if pm != nil {
		discoveryToken := identity.DeriveDiscoveryToken(networkID)
		pm.AddValidToken(discoveryToken)
		log.Debug("added network discovery token for new network")
	}

	identityPath := filepath.Join(n.config.Node.DataDir, identity.IdentityFileName)
	if err := id.Save(identityPath); err != nil {
		log.Warn("failed to save identity with network ID", "error", err)
	}

	log.Info("created new mesh network", "network_id", networkID)
	return nil
}

// generateInvite creates a new invite with the specified options.
func (n *Node) generateInvite(id *identity.Identity, expiry time.Duration, maxUses int) (*identity.Invite, error) {
	opts := identity.InviteOptions{
		Expiry:  expiry,
		MaxUses: maxUses,
	}
	if opts.Expiry <= 0 {
		opts.Expiry = identity.DefaultInviteExpiry
	}
	if opts.MaxUses < 0 {
		opts.MaxUses = identity.DefaultMaxUses
	}

	invite, err := identity.NewInvite(id, opts)
	if err != nil {
		return nil, fmt.Errorf("generating invite: %w", err)
	}

	return invite, nil
}

// storeAndRegisterInvite stores the invite and registers its token with the peer manager.
func (n *Node) storeAndRegisterInvite(invStore *identity.InviteStore, pm *mesh.PeerManager, invite *identity.Invite) string {
	key := invStore.AddGenerated(invite)
	if err := invStore.Save(); err != nil {
		log.Warn("failed to persist invite store", "error", err)
	}

	if pm != nil {
		pm.AddValidToken(invite.AuthToken)
	}

	return key
}

// buildInviteResult creates the RPC result from an invite.
func (n *Node) buildInviteResult(invite *identity.Invite, key string) (*rpc.InviteCreateResult, error) {
	inviteCode, err := invite.Encode()
	if err != nil {
		return nil, fmt.Errorf("encoding invite: %w", err)
	}

	log.Info("invite created",
		"key", key[:16]+"...",
		"expires_at", invite.ExpiresAt,
		"max_uses", invite.MaxUses)

	return &rpc.InviteCreateResult{
		InviteCode: inviteCode,
		ExpiresAt:  invite.ExpiresAt.Format(time.RFC3339),
		MaxUses:    invite.MaxUses,
	}, nil
}

// handleTokenUsed is called when an auth token is successfully used to
// authenticate a peer. It atomically marks the corresponding invite as used
// and persists the change, enforcing MaxUses limits.
func (n *Node) handleTokenUsed(token []byte) {
	n.mu.RLock()
	invStore := n.inviteStore
	n.mu.RUnlock()

	if invStore == nil || len(token) == 0 {
		return
	}

	tokenKey := fmt.Sprintf("%x", token)

	// Atomically mark the invite as used - this is safe for concurrent calls.
	// UseGenerated handles the race condition where two peers might try to
	// use the same single-use invite simultaneously.
	remainingUses, found := invStore.UseGenerated(tokenKey)
	if !found {
		log.Debug("token used but no matching invite found (may be discovery token)")
		return
	}
	if remainingUses < 0 {
		log.Debug("invite already exhausted or invalid")
		return
	}

	// Persist the change
	if err := invStore.Save(); err != nil {
		log.Warn("failed to persist invite usage", "error", err)
	} else {
		log.Info("invite usage recorded",
			"token_prefix", tokenKey[:16]+"...",
			"uses_remaining", remainingUses)
	}

	// If the invite is now exhausted, remove the token from
	// valid tokens to prevent further handshake attempts
	if remainingUses <= 0 {
		log.Info("invite exhausted, removing token",
			"token_prefix", tokenKey[:16]+"...")
		n.peers.RemoveToken(token)
	}
}

// AcceptInvite accepts an invite code and initiates connection to the inviter.
func (n *Node) AcceptInvite(ctx context.Context, inviteCode string) (*rpc.InviteAcceptResult, error) {
	id, invStore, err := n.getIdentityAndStore()
	if err != nil {
		return nil, err
	}

	invite, err := n.parseAndValidateInvite(inviteCode)
	if err != nil {
		return nil, err
	}

	n.storeAndUpdateInvite(invite, id, invStore)

	if err := n.initiateInviteConnection(ctx, inviteCode, invite, invStore); err != nil {
		return nil, err
	}

	log.Info("invite accepted",
		"network_id", invite.NetworkID,
		"peer_dest", invite.I2PDest[:32]+"...")

	return &rpc.InviteAcceptResult{
		NetworkID:  invite.NetworkID,
		PeerNodeID: invite.CreatedBy,
		TunnelIP:   n.TunnelIP(),
		Message:    "Successfully accepted invite and initiated connection",
	}, nil
}

// getIdentityAndStore retrieves identity and invite store from the node.
func (n *Node) getIdentityAndStore() (*identity.Identity, *identity.InviteStore, error) {
	n.mu.RLock()
	id := n.identity
	invStore := n.inviteStore
	n.mu.RUnlock()

	if id == nil || invStore == nil {
		return nil, nil, errors.New("node not fully initialized")
	}
	return id, invStore, nil
}

// storeAndUpdateInvite stores the invite as pending and updates network ID.
func (n *Node) storeAndUpdateInvite(invite *identity.Invite, id *identity.Identity, invStore *identity.InviteStore) {
	invStore.AddPending(invite)
	if err := invStore.Save(); err != nil {
		log.Warn("failed to persist invite store", "error", err)
	}

	id.SetNetworkID(invite.NetworkID)

	n.mu.RLock()
	pm := n.peers
	n.mu.RUnlock()
	if pm != nil {
		discoveryToken := identity.DeriveDiscoveryToken(invite.NetworkID)
		pm.AddValidToken(discoveryToken)
		log.Debug("added network discovery token for joined network")
	}
}

// initiateInviteConnection connects to the peer and marks invite as accepted.
func (n *Node) initiateInviteConnection(ctx context.Context, inviteCode string, invite *identity.Invite, invStore *identity.InviteStore) error {
	_, err := n.ConnectPeer(ctx, inviteCode)
	if err != nil {
		return fmt.Errorf("connecting to peer: %w", err)
	}

	invStore.MarkAccepted(invite.NetworkID)
	if err := invStore.Save(); err != nil {
		log.Warn("failed to persist invite store", "error", err)
	}
	return nil
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
		// Truncate I2P destination for display (full dest is very long)
		i2pDest := r.I2PDest
		if len(i2pDest) > 52 {
			i2pDest = i2pDest[:52] + "..."
		}

		result[i] = rpc.RouteInfo{
			NodeID:      r.NodeID,
			TunnelIP:    r.TunnelIP.String(),
			HopCount:    r.HopCount,
			ViaNodeID:   r.ViaNodeID,
			LastSeen:    r.LastSeen.Format(time.RFC3339),
			WGPublicKey: r.WGPublicKey,
			I2PDest:     i2pDest,
			CreatedAt:   r.CreatedAt.Format(time.RFC3339),
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

	if err := n.validateSetConfigPrereqs(key); err != nil {
		return nil, err
	}

	oldValue, err := n.applyConfigValue(key, value)
	if err != nil {
		return nil, err
	}

	log.Info("configuration updated", "key", key, "old_value", oldValue, "new_value", value)
	return oldValue, nil
}

// validateSetConfigPrereqs checks if configuration can be modified.
func (n *Node) validateSetConfigPrereqs(key string) error {
	if n.config == nil {
		return errors.New("configuration not available")
	}
	if key == "" {
		return errors.New("key is required")
	}
	return nil
}

// applyConfigValue applies a configuration value and returns the old value.
func (n *Node) applyConfigValue(key string, value any) (any, error) {
	switch key {
	case "node.name":
		return n.setNodeName(value)
	case "mesh.max_peers":
		return n.setMeshMaxPeers(value)
	case "rpc.tcp_address":
		return n.setRPCTCPAddress(value)
	case "node.data_dir", "i2p.sam_address", "i2p.tunnel_length",
		"mesh.tunnel_subnet", "mesh.heartbeat_interval", "mesh.peer_timeout",
		"rpc.enabled", "rpc.socket", "web.enabled", "web.listen":
		return nil, fmt.Errorf("config key %s is read-only at runtime", key)
	default:
		return nil, fmt.Errorf("unknown config key: %s", key)
	}
}

// setNodeName sets the node name configuration.
func (n *Node) setNodeName(value any) (any, error) {
	oldValue := n.config.Node.Name
	if s, ok := value.(string); ok {
		n.config.Node.Name = s
		return oldValue, nil
	}
	return nil, errors.New("node.name must be a string")
}

// setMeshMaxPeers sets the mesh max peers configuration.
func (n *Node) setMeshMaxPeers(value any) (any, error) {
	oldValue := n.config.Mesh.MaxPeers
	switch v := value.(type) {
	case int:
		n.config.Mesh.MaxPeers = v
		return oldValue, nil
	case float64:
		n.config.Mesh.MaxPeers = int(v)
		return oldValue, nil
	default:
		return nil, errors.New("mesh.max_peers must be an integer")
	}
}

// setRPCTCPAddress sets the RPC TCP address configuration.
func (n *Node) setRPCTCPAddress(value any) (any, error) {
	oldValue := n.config.RPC.TCPAddress
	if s, ok := value.(string); ok {
		n.config.RPC.TCPAddress = s
		return oldValue, nil
	}
	return nil, errors.New("rpc.tcp_address must be a string")
}

// truncateDest shortens an I2P destination for logging.
func truncateDest(dest string) string {
	if len(dest) > 32 {
		return dest[:32] + "..."
	}
	return dest
}
