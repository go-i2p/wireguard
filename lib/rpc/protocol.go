// Package rpc provides JSON-RPC over Unix socket and TCP for i2plan.
// It exposes node status, peer management, invite operations, and
// configuration access to external clients.
package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Protocol version for compatibility checking.
const ProtocolVersion = "1.0"

// Standard error codes following JSON-RPC 2.0 conventions.
const (
	// Parse error - invalid JSON
	ErrCodeParse = -32700
	// Invalid request - not a valid Request object
	ErrCodeInvalidRequest = -32600
	// Method not found
	ErrCodeMethodNotFound = -32601
	// Invalid params
	ErrCodeInvalidParams = -32602
	// Internal error
	ErrCodeInternal = -32603
	// Authentication required
	ErrCodeAuthRequired = -32001
	// Permission denied
	ErrCodePermissionDenied = -32002
	// Not found (generic)
	ErrCodeNotFound = -32003
	// Rate limit exceeded
	ErrCodeRateLimited = -32004
)

// Request represents a JSON-RPC request.
type Request struct {
	// JSONRPC must be "2.0"
	JSONRPC string `json:"jsonrpc"`
	// Method is the RPC method name
	Method string `json:"method"`
	// Params are the method parameters (can be object or array)
	Params json.RawMessage `json:"params,omitempty"`
	// ID is the request identifier (can be string or number)
	ID json.RawMessage `json:"id,omitempty"`
}

// Response represents a JSON-RPC response.
type Response struct {
	// JSONRPC is always "2.0"
	JSONRPC string `json:"jsonrpc"`
	// Result is the method result (omitted on error)
	Result any `json:"result,omitempty"`
	// Error is the error object (omitted on success)
	Error *Error `json:"error,omitempty"`
	// ID matches the request ID
	ID json.RawMessage `json:"id,omitempty"`
}

// Error represents a JSON-RPC error.
type Error struct {
	// Code is the error code
	Code int `json:"code"`
	// Message is a short description
	Message string `json:"message"`
	// Data contains additional information
	Data any `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("%s (code %d): %v", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
}

// NewError creates a new Error with the given code and message.
func NewError(code int, message string, data any) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// NewErrorResponse creates a Response with an error.
func NewErrorResponse(id json.RawMessage, err *Error) *Response {
	return &Response{
		JSONRPC: "2.0",
		Error:   err,
		ID:      id,
	}
}

// NewSuccessResponse creates a Response with a result.
func NewSuccessResponse(id json.RawMessage, result any) *Response {
	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

// ValidateRequest checks that a Request is valid JSON-RPC 2.0.
func ValidateRequest(req *Request) error {
	if req.JSONRPC != "2.0" {
		return errors.New("jsonrpc must be \"2.0\"")
	}
	if req.Method == "" {
		return errors.New("method is required")
	}
	return nil
}

// Common error constructors.

// ErrMethodNotFound returns a method not found error.
func ErrMethodNotFound(method string) *Error {
	return NewError(ErrCodeMethodNotFound, "method not found", method)
}

// ErrInvalidParams returns an invalid parameters error.
func ErrInvalidParams(details string) *Error {
	return NewError(ErrCodeInvalidParams, "invalid params", details)
}

// ErrInternal returns an internal error.
func ErrInternal(details string) *Error {
	return NewError(ErrCodeInternal, "internal error", details)
}

// ErrAuthRequired returns an authentication required error.
func ErrAuthRequired() *Error {
	return NewError(ErrCodeAuthRequired, "authentication required", nil)
}

// ErrPermissionDenied returns a permission denied error.
func ErrPermissionDenied(details string) *Error {
	return NewError(ErrCodePermissionDenied, "permission denied", details)
}

// ErrNotFound returns a not found error.
func ErrNotFound(resource string) *Error {
	return NewError(ErrCodeNotFound, "not found", resource)
}

// ErrRateLimited returns a rate limit exceeded error.
func ErrRateLimited() *Error {
	return NewError(ErrCodeRateLimited, "rate limit exceeded", nil)
}

// ---- Request/Response types for each RPC method ----

// StatusResult is the response for "status" method.
type StatusResult struct {
	// NodeName is the configured node name
	NodeName string `json:"node_name"`
	// NodeID is the unique node identifier
	NodeID string `json:"node_id"`
	// State is the node state (running, stopped, etc.)
	State string `json:"state"`
	// TunnelIP is our mesh tunnel IP
	TunnelIP string `json:"tunnel_ip,omitempty"`
	// I2PDestination is our I2P destination (truncated for display)
	I2PDestination string `json:"i2p_destination,omitempty"`
	// PeerCount is the number of connected peers
	PeerCount int `json:"peer_count"`
	// Uptime is how long the node has been running
	Uptime string `json:"uptime"`
	// Version is the software version
	Version string `json:"version"`
}

// PeersListResult is the response for "peers.list" method.
type PeersListResult struct {
	Peers []PeerInfo `json:"peers"`
	Total int        `json:"total"`
}

// PeerInfo represents information about a peer.
type PeerInfo struct {
	// NodeID is the peer's unique identifier
	NodeID string `json:"node_id"`
	// TunnelIP is the peer's mesh tunnel IP
	TunnelIP string `json:"tunnel_ip"`
	// State is the connection state
	State string `json:"state"`
	// LastSeen is when we last heard from this peer
	LastSeen string `json:"last_seen"`
	// ConnectedAt is when the connection was established
	ConnectedAt string `json:"connected_at,omitempty"`
	// Latency is the round-trip time
	Latency string `json:"latency,omitempty"`
}

// PeersConnectParams is the request for "peers.connect" method.
type PeersConnectParams struct {
	// InviteCode is the invite code to use for connection
	InviteCode string `json:"invite_code"`
}

// PeersConnectResult is the response for "peers.connect" method.
type PeersConnectResult struct {
	// NodeID is the connected peer's node ID
	NodeID string `json:"node_id"`
	// TunnelIP is the connected peer's tunnel IP
	TunnelIP string `json:"tunnel_ip"`
	// Message is a status message
	Message string `json:"message"`
}

// InviteCreateParams is the request for "invite.create" method.
type InviteCreateParams struct {
	// Expiry is how long until the invite expires (e.g., "24h")
	Expiry string `json:"expiry,omitempty"`
	// MaxUses is the maximum number of uses
	MaxUses int `json:"max_uses,omitempty"`
}

// InviteCreateResult is the response for "invite.create" method.
type InviteCreateResult struct {
	// InviteCode is the generated invite code
	InviteCode string `json:"invite_code"`
	// ExpiresAt is when the invite expires
	ExpiresAt string `json:"expires_at"`
	// MaxUses is the maximum number of uses
	MaxUses int `json:"max_uses"`
}

// InviteAcceptParams is the request for "invite.accept" method.
type InviteAcceptParams struct {
	// InviteCode is the invite code to accept
	InviteCode string `json:"invite_code"`
}

// InviteAcceptResult is the response for "invite.accept" method.
type InviteAcceptResult struct {
	// NetworkID is the network we joined
	NetworkID string `json:"network_id"`
	// PeerNodeID is the inviter's node ID
	PeerNodeID string `json:"peer_node_id"`
	// TunnelIP is our allocated tunnel IP
	TunnelIP string `json:"tunnel_ip"`
	// Message is a status message
	Message string `json:"message"`
}

// RoutesListResult is the response for "routes.list" method.
type RoutesListResult struct {
	Routes []RouteInfo `json:"routes"`
	Total  int         `json:"total"`
}

// RouteInfo represents information about a route.
type RouteInfo struct {
	// TunnelIP is the destination IP
	TunnelIP string `json:"tunnel_ip"`
	// NodeID is the destination node ID
	NodeID string `json:"node_id"`
	// HopCount is the number of hops
	HopCount int `json:"hop_count"`
	// ViaNodeID is the next hop (empty for direct routes)
	ViaNodeID string `json:"via_node_id,omitempty"`
	// LastSeen is when this route was last confirmed
	LastSeen string `json:"last_seen"`
	// WGPublicKey is the WireGuard public key (base64)
	WGPublicKey string `json:"wg_public_key,omitempty"`
	// I2PDest is the I2P destination (truncated for display)
	I2PDest string `json:"i2p_dest,omitempty"`
	// CreatedAt is when this route was first learned
	CreatedAt string `json:"created_at,omitempty"`
}

// ConfigGetParams is the request for "config.get" method.
type ConfigGetParams struct {
	// Key is the config key to get (dot-separated, e.g., "mesh.max_peers")
	Key string `json:"key,omitempty"`
}

// ConfigGetResult is the response for "config.get" method.
type ConfigGetResult struct {
	// Key is the config key
	Key string `json:"key,omitempty"`
	// Value is the config value
	Value any `json:"value"`
}

// ConfigSetParams is the request for "config.set" method.
type ConfigSetParams struct {
	// Key is the config key to set
	Key string `json:"key"`
	// Value is the value to set
	Value any `json:"value"`
}

// ConfigSetResult is the response for "config.set" method.
type ConfigSetResult struct {
	// Key is the config key that was set
	Key string `json:"key"`
	// OldValue is the previous value
	OldValue any `json:"old_value,omitempty"`
	// NewValue is the new value
	NewValue any `json:"new_value"`
	// Message is a status message
	Message string `json:"message"`
}

// LogsListParams is the request for "logs.list" method.
type LogsListParams struct {
	// Level filters logs by minimum level (debug, info, warn, error)
	Level string `json:"level,omitempty"`
	// Limit is the maximum number of logs to return
	Limit int `json:"limit,omitempty"`
	// Since returns logs after this timestamp
	Since *time.Time `json:"since,omitempty"`
}

// LogsListResult is the response for "logs.list" method.
type LogsListResult struct {
	Logs  []LogEntry `json:"logs"`
	Total int        `json:"total"`
}

// LogEntry represents a single log entry.
type LogEntry struct {
	// Timestamp is when the log was recorded
	Timestamp time.Time `json:"timestamp"`
	// Level is the log level (debug, info, warn, error)
	Level string `json:"level"`
	// Message is the log message
	Message string `json:"message"`
	// Component is the component that generated the log
	Component string `json:"component,omitempty"`
}

// ---- Ban Management Types ----

// BanListParams is the request for "bans.list" method.
type BanListParams struct {
	// IncludeExpired includes expired bans in the result
	IncludeExpired bool `json:"include_expired,omitempty"`
}

// BanListResult is the response for "bans.list" method.
type BanListResult struct {
	Bans []BanInfo `json:"bans"`
}

// BanInfo represents a banned peer.
type BanInfo struct {
	// NodeID is the banned node's identifier
	NodeID string `json:"node_id"`
	// I2PDest is the banned node's I2P destination (if known)
	I2PDest string `json:"i2p_dest,omitempty"`
	// Reason is why the peer was banned
	Reason string `json:"reason"`
	// Description provides additional context
	Description string `json:"description,omitempty"`
	// BannedAt is when the ban was created
	BannedAt time.Time `json:"banned_at"`
	// ExpiresAt is when the ban expires (zero means permanent)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// StrikeCount is how many violations led to this ban
	StrikeCount int `json:"strike_count"`
}

// BanAddParams is the request for "bans.add" method.
type BanAddParams struct {
	// NodeID is the node to ban (required)
	NodeID string `json:"node_id"`
	// Reason is why the peer is being banned
	Reason string `json:"reason,omitempty"`
	// Description provides additional context
	Description string `json:"description,omitempty"`
	// Duration is how long the ban lasts (0 = permanent)
	Duration string `json:"duration,omitempty"`
}

// BanAddResult is the response for "bans.add" method.
type BanAddResult struct {
	// Success indicates if the ban was added
	Success bool `json:"success"`
	// Message provides additional context
	Message string `json:"message"`
}

// BanRemoveParams is the request for "bans.remove" method.
type BanRemoveParams struct {
	// NodeID is the node to unban
	NodeID string `json:"node_id"`
}

// BanRemoveResult is the response for "bans.remove" method.
type BanRemoveResult struct {
	// Success indicates if the ban was removed
	Success bool `json:"success"`
	// Message provides additional context
	Message string `json:"message"`
}
