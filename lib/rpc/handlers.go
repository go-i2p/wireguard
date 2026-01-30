package rpc

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-i2p/wireguard/lib/metrics"
	"github.com/go-i2p/wireguard/lib/ratelimit"
)

// NodeProvider provides access to node state for RPC handlers.
// This interface abstracts the node implementation to avoid circular imports.
type NodeProvider interface {
	// NodeName returns the configured node name.
	NodeName() string
	// NodeID returns the unique node identifier.
	NodeID() string
	// State returns the current node state as a string.
	State() string
	// TunnelIP returns our mesh tunnel IP.
	TunnelIP() string
	// I2PDestination returns our I2P destination.
	I2PDestination() string
	// StartTime returns when the node started.
	StartTime() time.Time
	// Version returns the software version.
	Version() string
}

// PeerProvider provides access to peer information for RPC handlers.
type PeerProvider interface {
	// ListPeers returns all known peers.
	ListPeers() []PeerInfo
	// ConnectPeer connects to a peer using an invite code.
	ConnectPeer(ctx context.Context, inviteCode string) (*PeersConnectResult, error)
}

// InviteProvider provides invite operations for RPC handlers.
type InviteProvider interface {
	// CreateInvite creates a new invite code.
	CreateInvite(expiry time.Duration, maxUses int) (*InviteCreateResult, error)
	// AcceptInvite accepts an invite code.
	AcceptInvite(ctx context.Context, inviteCode string) (*InviteAcceptResult, error)
}

// RouteProvider provides route information for RPC handlers.
type RouteProvider interface {
	// ListRoutes returns all routes.
	ListRoutes() []RouteInfo
}

// ConfigProvider provides configuration access for RPC handlers.
type ConfigProvider interface {
	// GetConfig returns the configuration value for a key.
	// If key is empty, returns the entire configuration.
	GetConfig(key string) (any, error)
	// SetConfig sets a configuration value.
	SetConfig(key string, value any) (oldValue any, err error)
}

// BanProvider provides ban list operations for RPC handlers.
type BanProvider interface {
	// ListBans returns all active bans.
	ListBans() []BanEntry
	// AddBan adds a peer to the ban list.
	AddBan(nodeID, reason, description string, duration time.Duration) error
	// RemoveBan removes a peer from the ban list.
	RemoveBan(nodeID string) bool
}

// BanEntry is used by BanProvider to return ban information.
type BanEntry struct {
	NodeID      string
	I2PDest     string
	Reason      string
	Description string
	BannedAt    time.Time
	ExpiresAt   time.Time
	StrikeCount int
}

// Handlers provides RPC handlers with access to the node.
type Handlers struct {
	node            NodeProvider
	peers           PeerProvider
	invite          InviteProvider
	routes          RouteProvider
	config          ConfigProvider
	bans            BanProvider
	inviteRateLimit *ratelimit.Limiter // Rate limiter for invite acceptance
}

// HandlersConfig configures the RPC handlers.
type HandlersConfig struct {
	Node   NodeProvider
	Peers  PeerProvider
	Invite InviteProvider
	Routes RouteProvider
	Config ConfigProvider
	Bans   BanProvider
}

// NewHandlers creates RPC handlers.
func NewHandlers(cfg HandlersConfig) *Handlers {
	return &Handlers{
		node:            cfg.Node,
		peers:           cfg.Peers,
		invite:          cfg.Invite,
		routes:          cfg.Routes,
		config:          cfg.Config,
		bans:            cfg.Bans,
		inviteRateLimit: ratelimit.New(0.1, 5), // 0.1/sec (1 per 10s), burst 5
	}
}

// RegisterAll registers all handlers with the server.
func (h *Handlers) RegisterAll(s *Server) {
	s.RegisterHandler("status", h.Status)
	s.RegisterHandler("peers.list", h.PeersList)
	s.RegisterHandler("peers.connect", h.PeersConnect)
	s.RegisterHandler("invite.create", h.InviteCreate)
	s.RegisterHandler("invite.accept", h.InviteAccept)
	s.RegisterHandler("routes.list", h.RoutesList)
	s.RegisterHandler("config.get", h.ConfigGet)
	s.RegisterHandler("config.set", h.ConfigSet)
	s.RegisterHandler("bans.list", h.BansList)
	s.RegisterHandler("bans.add", h.BansAdd)
	s.RegisterHandler("bans.remove", h.BansRemove)
}

// Status returns the node status.
func (h *Handlers) Status(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.node == nil {
		return nil, ErrInternal("node not available")
	}

	uptime := time.Since(h.node.StartTime())

	result := &StatusResult{
		NodeName:       h.node.NodeName(),
		NodeID:         h.node.NodeID(),
		State:          h.node.State(),
		TunnelIP:       h.node.TunnelIP(),
		I2PDestination: truncateI2PDest(h.node.I2PDestination()),
		Uptime:         formatDuration(uptime),
		Version:        h.node.Version(),
	}

	// Get peer count if peers provider available
	if h.peers != nil {
		result.PeerCount = len(h.peers.ListPeers())
	}

	return result, nil
}

// PeersList returns the list of known peers.
func (h *Handlers) PeersList(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.peers == nil {
		return nil, ErrInternal("peers not available")
	}

	peers := h.peers.ListPeers()
	return &PeersListResult{
		Peers: peers,
		Total: len(peers),
	}, nil
}

// PeersConnect connects to a peer using an invite code.
func (h *Handlers) PeersConnect(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.peers == nil {
		return nil, ErrInternal("peers not available")
	}

	var p PeersConnectParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ErrInvalidParams(err.Error())
	}
	if p.InviteCode == "" {
		return nil, ErrInvalidParams("invite_code is required")
	}

	result, err := h.peers.ConnectPeer(ctx, p.InviteCode)
	if err != nil {
		return nil, ErrInternal(err.Error())
	}

	return result, nil
}

// InviteCreate creates a new invite code.
func (h *Handlers) InviteCreate(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.invite == nil {
		return nil, ErrInternal("invites not available")
	}

	var p InviteCreateParams
	if params != nil {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ErrInvalidParams(err.Error())
		}
	}

	// Parse expiry
	expiry := 24 * time.Hour
	if p.Expiry != "" {
		d, err := time.ParseDuration(p.Expiry)
		if err != nil {
			return nil, ErrInvalidParams("invalid expiry duration: " + err.Error())
		}
		expiry = d
	}

	// Default max uses
	maxUses := 1
	if p.MaxUses > 0 {
		maxUses = p.MaxUses
	}

	result, err := h.invite.CreateInvite(expiry, maxUses)
	if err != nil {
		return nil, ErrInternal(err.Error())
	}

	return result, nil
}

// InviteAccept accepts an invite code.
func (h *Handlers) InviteAccept(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.invite == nil {
		return nil, ErrInternal("invites not available")
	}

	// Rate limit invite acceptance to prevent brute-force attacks
	if !h.inviteRateLimit.Allow() {
		metrics.RateLimitRejections.Inc()
		return nil, ErrRateLimited()
	}

	var p InviteAcceptParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ErrInvalidParams(err.Error())
	}
	if p.InviteCode == "" {
		return nil, ErrInvalidParams("invite_code is required")
	}

	result, err := h.invite.AcceptInvite(ctx, p.InviteCode)
	if err != nil {
		return nil, ErrInternal(err.Error())
	}

	return result, nil
}

// RoutesList returns the routing table.
func (h *Handlers) RoutesList(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.routes == nil {
		return nil, ErrInternal("routes not available")
	}

	routes := h.routes.ListRoutes()
	return &RoutesListResult{
		Routes: routes,
		Total:  len(routes),
	}, nil
}

// ConfigGet returns configuration values.
func (h *Handlers) ConfigGet(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.config == nil {
		return nil, ErrInternal("config not available")
	}

	var p ConfigGetParams
	if params != nil {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ErrInvalidParams(err.Error())
		}
	}

	value, err := h.config.GetConfig(p.Key)
	if err != nil {
		return nil, ErrNotFound(p.Key)
	}

	return &ConfigGetResult{
		Key:   p.Key,
		Value: value,
	}, nil
}

// ConfigSet sets a configuration value.
func (h *Handlers) ConfigSet(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.config == nil {
		return nil, ErrInternal("config not available")
	}

	var p ConfigSetParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ErrInvalidParams(err.Error())
	}
	if p.Key == "" {
		return nil, ErrInvalidParams("key is required")
	}

	oldValue, err := h.config.SetConfig(p.Key, p.Value)
	if err != nil {
		return nil, ErrInternal(err.Error())
	}

	return &ConfigSetResult{
		Key:      p.Key,
		OldValue: oldValue,
		NewValue: p.Value,
		Message:  "configuration updated",
	}, nil
}

// Helper functions

// truncateI2PDest truncates an I2P destination for display.
func truncateI2PDest(dest string) string {
	if len(dest) <= 16 {
		return dest
	}
	return dest[:8] + "..." + dest[len(dest)-8:]
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours < 24 {
		return d.Round(time.Minute).String()
	}
	days := hours / 24
	hours = hours % 24
	return formatDays(days, hours, minutes)
}

// formatDays formats days, hours, minutes for display.
func formatDays(days, hours, minutes int) string {
	if days == 1 {
		return "1 day"
	}
	if hours == 0 && minutes == 0 {
		return formatPlural(days, "day", "days")
	}
	return formatPlural(days, "day", "days") + " " + formatPlural(hours, "hour", "hours")
}

// formatPlural formats a number with singular/plural form.
func formatPlural(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return formatInt(n) + " " + plural
}

// formatInt formats an integer as a string.
func formatInt(n int) string {
	return strconv.Itoa(n)
}

// ---- Ban Handlers ----

// BansList returns all banned peers.
func (h *Handlers) BansList(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.bans == nil {
		return nil, ErrInternal("bans not available")
	}

	bans := h.bans.ListBans()
	result := &BanListResult{
		Bans: make([]BanInfo, 0, len(bans)),
	}

	for _, b := range bans {
		info := BanInfo{
			NodeID:      b.NodeID,
			I2PDest:     b.I2PDest,
			Reason:      b.Reason,
			Description: b.Description,
			BannedAt:    b.BannedAt,
			StrikeCount: b.StrikeCount,
		}
		if !b.ExpiresAt.IsZero() {
			info.ExpiresAt = &b.ExpiresAt
		}
		result.Bans = append(result.Bans, info)
	}

	return result, nil
}

// BansAdd bans a peer.
// parseBanDuration parses the duration string, returning zero duration if empty.
func parseBanDuration(durationStr string) (time.Duration, error) {
	if durationStr == "" {
		return 0, nil
	}
	return time.ParseDuration(durationStr)
}

// formatBanMessage creates the result message for a ban operation.
func formatBanMessage(duration time.Duration) string {
	if duration > 0 {
		return "peer banned for " + duration.String()
	}
	return "peer banned permanently"
}

func (h *Handlers) BansAdd(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.bans == nil {
		return nil, ErrInternal("bans not available")
	}

	var p BanAddParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ErrInvalidParams(err.Error())
	}
	if p.NodeID == "" {
		return nil, ErrInvalidParams("node_id is required")
	}

	reason := p.Reason
	if reason == "" {
		reason = "manual"
	}

	duration, err := parseBanDuration(p.Duration)
	if err != nil {
		return nil, ErrInvalidParams("invalid duration: " + err.Error())
	}

	if err := h.bans.AddBan(p.NodeID, reason, p.Description, duration); err != nil {
		return nil, ErrInternal(err.Error())
	}

	return &BanAddResult{
		Success: true,
		Message: formatBanMessage(duration),
	}, nil
}

// BansRemove unbans a peer.
func (h *Handlers) BansRemove(ctx context.Context, params json.RawMessage) (any, *Error) {
	if h.bans == nil {
		return nil, ErrInternal("bans not available")
	}

	var p BanRemoveParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ErrInvalidParams(err.Error())
	}
	if p.NodeID == "" {
		return nil, ErrInvalidParams("node_id is required")
	}

	if !h.bans.RemoveBan(p.NodeID) {
		return &BanRemoveResult{
			Success: false,
			Message: "peer not found in ban list",
		}, nil
	}

	return &BanRemoveResult{
		Success: true,
		Message: "peer unbanned",
	}, nil
}
