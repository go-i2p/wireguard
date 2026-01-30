package web

import (
	"context"

	"github.com/go-i2p/wireguard/lib/rpc"
)

// RPCClient defines the interface for RPC operations used by the web server.
// This interface allows for easier testing by enabling mock implementations.
type RPCClient interface {
	// Status returns the current node status.
	Status(ctx context.Context) (*rpc.StatusResult, error)
	// PeersList returns the list of peers.
	PeersList(ctx context.Context) (*rpc.PeersListResult, error)
	// RoutesList returns the list of routes.
	RoutesList(ctx context.Context) (*rpc.RoutesListResult, error)
	// ConfigGet returns the configuration value for the given key.
	ConfigGet(ctx context.Context, key string) (*rpc.ConfigGetResult, error)
	// InviteCreate creates a new invite code.
	InviteCreate(ctx context.Context, expiry string, maxUses int) (*rpc.InviteCreateResult, error)
	// InviteAccept accepts an invite code.
	InviteAccept(ctx context.Context, inviteCode string) (*rpc.InviteAcceptResult, error)
	// Close closes the RPC connection.
	Close() error
}

// Verify that *rpc.Client implements RPCClient at compile time.
var _ RPCClient = (*rpc.Client)(nil)
