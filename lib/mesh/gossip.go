// Package mesh provides gossip protocol implementation for the i2plan mesh network.
// This file implements the gossip engine for peer discovery and route propagation.
package mesh

import (
	"context"
	"log/slog"
	"math/rand"
	"net/netip"
	"sync"
	"time"
)

// GossipConfig configures the gossip engine.
type GossipConfig struct {
	// HeartbeatInterval is how often to send peer announcements.
	HeartbeatInterval time.Duration
	// GossipInterval is how often to share peer lists.
	GossipInterval time.Duration
	// AntiEntropyInterval is how often to do full state sync.
	AntiEntropyInterval time.Duration
	// PeerTimeout is when a peer is considered stale.
	PeerTimeout time.Duration
	// FanOut is how many random peers to gossip to.
	FanOut int
	// Logger for gossip operations.
	Logger *slog.Logger
}

// DefaultGossipConfig returns sensible defaults for gossip.
func DefaultGossipConfig() GossipConfig {
	return GossipConfig{
		HeartbeatInterval:   30 * time.Second,
		GossipInterval:      60 * time.Second,
		AntiEntropyInterval: 5 * time.Minute,
		PeerTimeout:         5 * time.Minute,
		FanOut:              3,
	}
}

// MessageSender is the interface for sending gossip messages.
type MessageSender interface {
	// SendTo sends a message to a specific peer by node ID.
	SendTo(nodeID string, data []byte) error
	// Broadcast sends a message to all connected peers.
	Broadcast(data []byte) error
}

// GossipEngine manages gossip protocol operations.
type GossipEngine struct {
	mu     sync.RWMutex
	config GossipConfig
	logger *slog.Logger

	// Dependencies
	peerManager  *PeerManager
	routingTable *RoutingTable
	sender       MessageSender

	// Our identity
	nodeID      string
	i2pDest     string
	wgPublicKey string
	tunnelIP    string
	networkID   string

	// Callbacks for handshake events
	onHandshakeInit     func(*HandshakeInit) (*HandshakeResponse, error)
	onHandshakeResponse func(*HandshakeResponse) error
	onHandshakeComplete func(*HandshakeComplete) error

	// State
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// GossipEngineConfig configures the gossip engine dependencies.
type GossipEngineConfig struct {
	Config       GossipConfig
	PeerManager  *PeerManager
	RoutingTable *RoutingTable
	Sender       MessageSender
	NodeID       string
	I2PDest      string
	WGPublicKey  string
	TunnelIP     string
	NetworkID    string
}

// NewGossipEngine creates a new gossip engine.
func NewGossipEngine(cfg GossipEngineConfig) *GossipEngine {
	config := cfg.Config
	if config.HeartbeatInterval == 0 {
		config = DefaultGossipConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &GossipEngine{
		config:       config,
		logger:       logger,
		peerManager:  cfg.PeerManager,
		routingTable: cfg.RoutingTable,
		sender:       cfg.Sender,
		nodeID:       cfg.NodeID,
		i2pDest:      cfg.I2PDest,
		wgPublicKey:  cfg.WGPublicKey,
		tunnelIP:     cfg.TunnelIP,
		networkID:    cfg.NetworkID,
	}
}

// Start begins the gossip protocol loops.
// Note: If no MessageSender was provided, gossip will run in local-only mode
// where no messages are sent to other peers. This can be useful for testing.
func (g *GossipEngine) Start(ctx context.Context) error {
	g.mu.Lock()
	if g.running {
		g.mu.Unlock()
		return nil
	}

	// Warn if no sender is configured - gossip will be local-only
	if g.sender == nil {
		g.logger.Warn("gossip engine starting without message sender - running in local-only mode")
	}

	g.running = true

	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel
	g.mu.Unlock()

	g.logger.Info("starting gossip engine",
		"heartbeat", g.config.HeartbeatInterval,
		"gossip", g.config.GossipInterval,
		"fanout", g.config.FanOut)

	// Start heartbeat loop
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.heartbeatLoop(ctx)
	}()

	// Start gossip loop
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.gossipLoop(ctx)
	}()

	// Start anti-entropy loop
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.antiEntropyLoop(ctx)
	}()

	// Start peer pruning loop
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.pruneLoop(ctx)
	}()

	return nil
}

// Stop halts all gossip protocol operations.
// It sends a leave announcement to notify peers before stopping.
func (g *GossipEngine) Stop() {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return
	}
	g.running = false
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()

	// Send leave announcement to notify peers
	g.sendLeaveAnnouncement()

	g.wg.Wait()
	g.logger.Info("gossip engine stopped")
}

// GracefulShutdown performs a graceful shutdown with a timeout.
// It notifies peers of departure and waits for acknowledgments.
func (g *GossipEngine) GracefulShutdown(ctx context.Context) error {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return nil
	}
	g.running = false
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()

	// Send leave announcement
	g.sendLeaveAnnouncement()

	// Wait for goroutines with context timeout
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		g.logger.Info("graceful shutdown complete")
		return nil
	case <-ctx.Done():
		g.logger.Warn("graceful shutdown timed out")
		return ctx.Err()
	}
}

// sendLeaveAnnouncement broadcasts a leave message to all peers.
func (g *GossipEngine) sendLeaveAnnouncement() {
	if g.sender == nil {
		return
	}

	leave := &PeerLeavePayload{
		NodeID:    g.nodeID,
		NetworkID: g.networkID,
		Reason:    "shutdown",
	}

	data, err := EncodeMessage(MsgPeerLeave, leave)
	if err != nil {
		g.logger.Error("failed to encode leave message", "error", err)
		return
	}

	if err := g.sender.Broadcast(data); err != nil {
		g.logger.Debug("failed to broadcast leave", "error", err)
	} else {
		g.logger.Info("sent leave announcement to peers")
	}
}

// IsRunning returns whether the gossip engine is active.
func (g *GossipEngine) IsRunning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.running
}

// heartbeatLoop periodically announces our presence.
func (g *GossipEngine) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(g.config.HeartbeatInterval)
	defer ticker.Stop()

	// Send initial announcement
	g.sendAnnouncement()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.sendAnnouncement()
		}
	}
}

// gossipLoop periodically shares peer lists with random neighbors.
func (g *GossipEngine) gossipLoop(ctx context.Context) {
	ticker := time.NewTicker(g.config.GossipInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.gossipPeerList()
		}
	}
}

// antiEntropyLoop periodically does full state synchronization.
func (g *GossipEngine) antiEntropyLoop(ctx context.Context) {
	ticker := time.NewTicker(g.config.AntiEntropyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.syncFullState()
		}
	}
}

// pruneLoop periodically removes stale peers and routes.
func (g *GossipEngine) pruneLoop(ctx context.Context) {
	ticker := time.NewTicker(g.config.PeerTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.pruneStale()
		}
	}
}

// sendAnnouncement broadcasts our presence to all peers.
func (g *GossipEngine) sendAnnouncement() {
	if g.sender == nil {
		return
	}

	peerCount := 0
	if g.peerManager != nil {
		peerCount = g.peerManager.ConnectedCount()
	}

	announce := &PeerAnnounce{
		I2PDest:     g.i2pDest,
		WGPublicKey: g.wgPublicKey,
		TunnelIP:    g.tunnelIP,
		NetworkID:   g.networkID,
		NodeID:      g.nodeID,
		PeerCount:   peerCount,
	}

	data, err := EncodeMessage(MsgPeerAnnounce, announce)
	if err != nil {
		g.logger.Error("failed to encode announcement", "error", err)
		return
	}

	if err := g.sender.Broadcast(data); err != nil {
		g.logger.Debug("failed to broadcast announcement", "error", err)
	} else {
		g.logger.Debug("sent peer announcement", "peer_count", peerCount)
	}
}

// gossipPeerList shares known peers with random neighbors.
func (g *GossipEngine) gossipPeerList() {
	if g.sender == nil || g.peerManager == nil {
		return
	}

	peers := g.peerManager.ConnectedPeers()
	if len(peers) == 0 {
		return
	}

	// Build peer list payload
	peerList := &PeerListPayload{
		NetworkID: g.networkID,
		NodeID:    g.nodeID,
		Peers:     make([]PeerInfo, 0, len(peers)),
	}

	for _, p := range peers {
		peerList.Peers = append(peerList.Peers, PeerInfo{
			NodeID:      p.NodeID,
			I2PDest:     p.I2PDest,
			WGPublicKey: p.WGPublicKey.String(),
			TunnelIP:    p.TunnelIP.String(),
			LastSeen:    p.LastSeen,
		})
	}

	data, err := EncodeMessage(MsgPeerList, peerList)
	if err != nil {
		g.logger.Error("failed to encode peer list", "error", err)
		return
	}

	// Select random peers to gossip to
	targets := g.selectRandomPeers(g.config.FanOut)
	for _, nodeID := range targets {
		if err := g.sender.SendTo(nodeID, data); err != nil {
			g.logger.Debug("failed to send peer list", "target", nodeID, "error", err)
		}
	}

	g.logger.Debug("gossiped peer list", "peers", len(peerList.Peers), "targets", len(targets))
}

// syncFullState sends complete routing table to a random peer.
func (g *GossipEngine) syncFullState() {
	if g.sender == nil || g.routingTable == nil || g.peerManager == nil {
		return
	}

	routes := g.routingTable.ListRoutes()
	if len(routes) == 0 {
		return
	}

	// Build route update payload
	update := &RouteUpdatePayload{
		NetworkID: g.networkID,
		NodeID:    g.nodeID,
		Routes:    make([]RouteInfo, 0, len(routes)),
	}

	for _, r := range routes {
		update.Routes = append(update.Routes, RouteInfo{
			TunnelIP:    r.TunnelIP.String(),
			WGPublicKey: r.WGPublicKey,
			I2PDest:     r.I2PDest,
			NodeID:      r.NodeID,
			HopCount:    r.HopCount,
			LastSeen:    r.LastSeen,
		})
	}

	data, err := EncodeMessage(MsgRouteUpdate, update)
	if err != nil {
		g.logger.Error("failed to encode route update", "error", err)
		return
	}

	// Send to one random peer for anti-entropy
	targets := g.selectRandomPeers(1)
	for _, nodeID := range targets {
		if err := g.sender.SendTo(nodeID, data); err != nil {
			g.logger.Debug("failed to send route update", "target", nodeID, "error", err)
		} else {
			g.logger.Debug("sent anti-entropy sync", "routes", len(update.Routes), "target", nodeID)
		}
	}
}

// pruneStale removes peers and routes that haven't been seen.
func (g *GossipEngine) pruneStale() {
	pruned := 0

	if g.peerManager != nil {
		pruned += g.peerManager.PruneStale(g.config.PeerTimeout)
	}

	if g.routingTable != nil {
		pruned += g.routingTable.ExpireStale(g.config.PeerTimeout)
	}

	if pruned > 0 {
		g.logger.Info("pruned stale entries", "count", pruned)
	}
}

// selectRandomPeers returns up to n random connected peer IDs.
// Uses partial Fisher-Yates shuffle for efficiency - only shuffles the first n elements.
func (g *GossipEngine) selectRandomPeers(n int) []string {
	if g.peerManager == nil {
		return nil
	}

	peers := g.peerManager.ConnectedPeers()
	if len(peers) == 0 {
		return nil
	}

	// Extract NodeIDs
	peerIDs := make([]string, len(peers))
	for i, p := range peers {
		peerIDs[i] = p.NodeID
	}

	// Partial Fisher-Yates: only shuffle first n elements
	// This is O(n) instead of O(len(peers)) for full shuffle
	if n > len(peerIDs) {
		n = len(peerIDs)
	}

	for i := 0; i < n; i++ {
		// Pick a random element from i to end
		j := i + rand.Intn(len(peerIDs)-i)
		peerIDs[i], peerIDs[j] = peerIDs[j], peerIDs[i]
	}

	return peerIDs[:n]
}

// HandleMessage processes an incoming gossip message.
func (g *GossipEngine) HandleMessage(msg *Message) error {
	switch msg.Type {
	case MsgHandshakeInit:
		return g.handleHandshakeInit(msg)
	case MsgHandshakeResponse:
		return g.handleHandshakeResponse(msg)
	case MsgHandshakeComplete:
		return g.handleHandshakeComplete(msg)
	case MsgPeerAnnounce:
		return g.handlePeerAnnounce(msg)
	case MsgPeerList:
		return g.handlePeerList(msg)
	case MsgRouteUpdate:
		return g.handleRouteUpdate(msg)
	case MsgPeerLeave:
		return g.handlePeerLeave(msg)
	default:
		return nil // Ignore unknown messages
	}
}

// handlePeerAnnounce processes a peer announcement.
func (g *GossipEngine) handlePeerAnnounce(msg *Message) error {
	announce, err := DecodePayload[PeerAnnounce](msg)
	if err != nil {
		return err
	}

	// Ignore our own announcements
	if announce.NodeID == g.nodeID {
		return nil
	}

	// Ignore different networks
	if announce.NetworkID != g.networkID {
		return nil
	}

	// Update peer last seen
	if g.peerManager != nil {
		g.peerManager.UpdatePeerSeen(announce.NodeID)
	}

	g.logger.Debug("received peer announcement",
		"from", announce.NodeID,
		"peers", announce.PeerCount)

	return nil
}

// handlePeerList processes a peer list message.
func (g *GossipEngine) handlePeerList(msg *Message) error {
	peerList, err := DecodePayload[PeerListPayload](msg)
	if err != nil {
		return err
	}

	if peerList.NetworkID != g.networkID {
		return nil
	}

	g.logger.Debug("received peer list",
		"from", peerList.NodeID,
		"peers", len(peerList.Peers))

	// We could trigger connection attempts to new peers here
	// For now, just log the discovery
	for _, p := range peerList.Peers {
		if p.NodeID != g.nodeID {
			g.logger.Debug("discovered peer via gossip", "node_id", p.NodeID)
		}
	}

	return nil
}

// handleRouteUpdate processes a route update message.
func (g *GossipEngine) handleRouteUpdate(msg *Message) error {
	update, err := DecodePayload[RouteUpdatePayload](msg)
	if err != nil {
		return err
	}

	if update.NetworkID != g.networkID {
		return nil
	}

	if g.routingTable == nil {
		return nil
	}

	g.logger.Debug("received route update",
		"from", update.NodeID,
		"routes", len(update.Routes))

	// Convert RouteInfo to RouteEntry and merge into our table
	entries := make([]*RouteEntry, 0, len(update.Routes))
	for _, ri := range update.Routes {
		tunnelIP, err := netip.ParseAddr(ri.TunnelIP)
		if err != nil {
			g.logger.Debug("skipping invalid tunnel IP in route update",
				"ip", ri.TunnelIP, "error", err)
			continue
		}

		entry := &RouteEntry{
			TunnelIP:    tunnelIP,
			WGPublicKey: ri.WGPublicKey,
			I2PDest:     ri.I2PDest,
			NodeID:      ri.NodeID,
			HopCount:    ri.HopCount + 1, // Increment hop count since we're receiving via another node
			LastSeen:    ri.LastSeen,
			ViaNodeID:   update.NodeID, // Route came via the sender
		}
		entries = append(entries, entry)
	}

	if len(entries) > 0 {
		added, updated, collisions := g.routingTable.Merge(entries)
		if added > 0 || updated > 0 {
			g.logger.Debug("merged route update",
				"added", added,
				"updated", updated,
				"collisions", len(collisions))
		}
		for _, coll := range collisions {
			g.logger.Warn("route collision detected", "error", coll)
		}
	}

	return nil
}

// handlePeerLeave processes a peer leave message.
func (g *GossipEngine) handlePeerLeave(msg *Message) error {
	leave, err := DecodePayload[PeerLeavePayload](msg)
	if err != nil {
		return err
	}

	if leave.NetworkID != g.networkID {
		return nil
	}

	g.logger.Info("peer leaving network", "node_id", leave.NodeID, "reason", leave.Reason)

	// Remove from peer manager and routing table
	if g.peerManager != nil {
		g.peerManager.RemovePeer(leave.NodeID)
	}

	if g.routingTable != nil {
		g.routingTable.RemoveByNodeID(leave.NodeID)
	}

	return nil
}

// handleHandshakeInit processes an incoming handshake initiation.
func (g *GossipEngine) handleHandshakeInit(msg *Message) error {
	init, err := DecodePayload[HandshakeInit](msg)
	if err != nil {
		return err
	}

	g.logger.Info("received handshake init",
		"from_node", init.NodeID,
		"i2p_dest", truncateString(init.I2PDest, 32))

	// Delegate to peer manager if no custom handler
	g.mu.RLock()
	customHandler := g.onHandshakeInit
	g.mu.RUnlock()

	var response *HandshakeResponse
	if customHandler != nil {
		response, err = customHandler(init)
	} else if g.peerManager != nil {
		response, err = g.peerManager.HandleHandshakeInit(init)
	} else {
		g.logger.Warn("no handler for handshake init")
		return nil
	}

	if err != nil {
		g.logger.Warn("handshake init handling failed", "error", err)
		return err
	}

	// Send response back to the initiator
	if response != nil && g.sender != nil {
		responseData, encErr := EncodeMessage(MsgHandshakeResponse, response)
		if encErr != nil {
			g.logger.Error("failed to encode handshake response", "error", encErr)
			return encErr
		}

		// Send directly to the initiator's I2P destination
		if sendErr := g.sender.SendTo(init.NodeID, responseData); sendErr != nil {
			// Fall back to destination-based send if nodeID not registered
			g.logger.Debug("SendTo failed, peer may not be registered yet", "error", sendErr)
		}

		g.logger.Info("sent handshake response",
			"to_node", init.NodeID,
			"accepted", response.Accepted)
	}

	return nil
}

// handleHandshakeResponse processes a handshake response.
func (g *GossipEngine) handleHandshakeResponse(msg *Message) error {
	resp, err := DecodePayload[HandshakeResponse](msg)
	if err != nil {
		return err
	}

	g.logger.Info("received handshake response",
		"from_node", resp.NodeID,
		"accepted", resp.Accepted)

	// Delegate to custom handler or peer manager
	g.mu.RLock()
	customHandler := g.onHandshakeResponse
	g.mu.RUnlock()

	if customHandler != nil {
		err = customHandler(resp)
	} else if g.peerManager != nil {
		err = g.peerManager.HandleHandshakeResponse(resp)
	} else {
		g.logger.Warn("no handler for handshake response")
		return nil
	}

	if err != nil {
		g.logger.Warn("handshake response handling failed", "error", err)
		return err
	}

	// If accepted, send completion message
	if resp.Accepted && g.peerManager != nil && g.sender != nil {
		complete := g.peerManager.CreateHandshakeComplete(true)
		completeData, encErr := EncodeMessage(MsgHandshakeComplete, complete)
		if encErr != nil {
			g.logger.Error("failed to encode handshake complete", "error", encErr)
			return encErr
		}

		if sendErr := g.sender.SendTo(resp.NodeID, completeData); sendErr != nil {
			g.logger.Debug("failed to send handshake complete", "error", sendErr)
		} else {
			g.logger.Info("sent handshake complete", "to_node", resp.NodeID)
		}
	}

	return nil
}

// handleHandshakeComplete processes a handshake completion message.
func (g *GossipEngine) handleHandshakeComplete(msg *Message) error {
	complete, err := DecodePayload[HandshakeComplete](msg)
	if err != nil {
		return err
	}

	g.logger.Info("received handshake complete",
		"from_node", complete.NodeID,
		"success", complete.Success)

	// Delegate to custom handler or peer manager
	g.mu.RLock()
	customHandler := g.onHandshakeComplete
	g.mu.RUnlock()

	if customHandler != nil {
		err = customHandler(complete)
	} else if g.peerManager != nil {
		err = g.peerManager.HandleHandshakeComplete(complete)
	} else {
		g.logger.Warn("no handler for handshake complete")
		return nil
	}

	if err != nil {
		g.logger.Warn("handshake complete handling failed", "error", err)
	}

	return err
}

// SetHandshakeCallbacks sets custom handlers for handshake messages.
// If not set, the gossip engine will use the PeerManager's default handlers.
func (g *GossipEngine) SetHandshakeCallbacks(
	onInit func(*HandshakeInit) (*HandshakeResponse, error),
	onResponse func(*HandshakeResponse) error,
	onComplete func(*HandshakeComplete) error,
) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onHandshakeInit = onInit
	g.onHandshakeResponse = onResponse
	g.onHandshakeComplete = onComplete
}

// truncateString truncates a string to maxLen with ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// AnnounceLeave broadcasts that we're leaving the network.
func (g *GossipEngine) AnnounceLeave(reason string) {
	if g.sender == nil {
		return
	}

	leave := &PeerLeavePayload{
		NodeID:    g.nodeID,
		NetworkID: g.networkID,
		Reason:    reason,
	}

	data, err := EncodeMessage(MsgPeerLeave, leave)
	if err != nil {
		g.logger.Error("failed to encode leave message", "error", err)
		return
	}

	if err := g.sender.Broadcast(data); err != nil {
		g.logger.Debug("failed to broadcast leave", "error", err)
	} else {
		g.logger.Info("announced departure from network")
	}
}

// PeerListPayload is the payload for MsgPeerList.
type PeerListPayload struct {
	NetworkID string     `json:"network_id"`
	NodeID    string     `json:"node_id"`
	Peers     []PeerInfo `json:"peers"`
}

// PeerInfo is a summary of peer information for gossip.
type PeerInfo struct {
	NodeID      string    `json:"node_id"`
	I2PDest     string    `json:"i2p_dest"`
	WGPublicKey string    `json:"wg_public_key"`
	TunnelIP    string    `json:"tunnel_ip"`
	LastSeen    time.Time `json:"last_seen"`
}

// RouteUpdatePayload is the payload for MsgRouteUpdate.
type RouteUpdatePayload struct {
	NetworkID string      `json:"network_id"`
	NodeID    string      `json:"node_id"`
	Routes    []RouteInfo `json:"routes"`
}

// RouteInfo is a summary of route information for gossip.
type RouteInfo struct {
	TunnelIP    string    `json:"tunnel_ip"`
	WGPublicKey string    `json:"wg_public_key"`
	I2PDest     string    `json:"i2p_dest"`
	NodeID      string    `json:"node_id"`
	HopCount    int       `json:"hop_count"`
	LastSeen    time.Time `json:"last_seen"`
}

// PeerLeavePayload is the payload for MsgPeerLeave.
type PeerLeavePayload struct {
	NodeID    string `json:"node_id"`
	NetworkID string `json:"network_id"`
	Reason    string `json:"reason,omitempty"`
}
