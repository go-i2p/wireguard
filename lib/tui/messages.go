package tui

import (
	"time"

	"github.com/go-i2p/wireguard/lib/rpc"
)

// refreshMsg contains refreshed data from RPC.
type refreshMsg struct {
	status *rpc.StatusResult
	peers  *rpc.PeersListResult
	routes *rpc.RoutesListResult
	err    error
}

// tickMsg triggers a data refresh.
type tickMsg time.Time

// inviteCreatedMsg indicates a new invite was created.
type inviteCreatedMsg struct {
	invite *rpc.InviteCreateResult
}

// inviteAcceptedMsg indicates an invite was accepted.
type inviteAcceptedMsg struct {
	result *rpc.InviteAcceptResult
}

// errMsg represents an error that occurred.
type errMsg struct {
	err error
}

// cleanupMsg signals that cleanup should occur before exit.
type cleanupMsg struct{}

// reconnectMsg indicates RPC needs reconnection.
type reconnectMsg struct {
	err error
}
