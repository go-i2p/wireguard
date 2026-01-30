package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-i2p/wireguard/lib/rpc"
)

// PeersModel is the model for the peers view.
type PeersModel struct {
	peers    *rpc.PeersListResult
	cursor   int
	width    int
	height   int
	showHelp bool
}

// NewPeersModel creates a new peers view model.
func NewPeersModel() PeersModel {
	return PeersModel{}
}

// SetData updates the peers data and manages cursor position.
// The cursor is preserved when possible to maintain user context, and only
// reset if it becomes out of bounds after a data update.
func (m *PeersModel) SetData(peers *rpc.PeersListResult) {
	oldPeers := m.peers
	m.peers = peers

	// Smart cursor management (Issue #9)
	if m.peers == nil || len(m.peers.Peers) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.peers.Peers) {
		// Only reset if out of bounds, preserve position otherwise
		m.cursor = len(m.peers.Peers) - 1
	}
	// If list size is same or similar, preserve cursor position
	_ = oldPeers // Avoid unused variable warning
}

// SetDimensions sets the view dimensions.
func (m *PeersModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages for the peers view.
func (m PeersModel) Update(msg tea.KeyMsg, client *rpc.Client) (PeersModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.peers != nil && m.cursor < len(m.peers.Peers)-1 {
			m.cursor++
		}
	}
	return m, nil
}

// View renders the peers view.
func (m PeersModel) View() string {
	if m.peers == nil {
		return styles.Muted.Render("Loading peers...")
	}

	if len(m.peers.Peers) == 0 {
		return m.renderEmptyState()
	}

	var b strings.Builder

	// Table header
	header := fmt.Sprintf("%-20s %-16s %-12s %-20s", "NODE ID", "TUNNEL IP", "STATE", "LAST SEEN")
	b.WriteString(styles.TableHeader.Render(header))
	b.WriteString("\n")

	// Table rows
	for i, peer := range m.peers.Peers {
		nodeID := truncate(peer.NodeID, 20)
		state := PeerStateStyle(peer.State).Render(peer.State)

		row := fmt.Sprintf("%-20s %-16s %-12s %-20s",
			nodeID,
			peer.TunnelIP,
			state,
			peer.LastSeen,
		)

		if i == m.cursor {
			row = styles.Selected.Render(row)
		} else {
			row = styles.TableRow.Render(row)
		}

		b.WriteString(row)
		b.WriteString("\n")
	}

	// Summary
	b.WriteString("\n")
	summary := fmt.Sprintf("Total: %d peers", m.peers.Total)
	b.WriteString(styles.Muted.Render(summary))

	return b.String()
}

// renderEmptyState renders the empty state for no peers.
func (m PeersModel) renderEmptyState() string {
	return renderEmptyState(
		m.width,
		m.height,
		"No Peers Connected",
		"Generate an invite code and share it with someone to connect.",
		[]string{"Press Tab → Invites → n to create invite"},
	)
}

// SelectedPeer returns the currently selected peer, or nil if no peer is selected
// or if the peers list is empty.
func (m PeersModel) SelectedPeer() *rpc.PeerInfo {
	if m.peers == nil || len(m.peers.Peers) == 0 {
		return nil
	}
	if m.cursor >= 0 && m.cursor < len(m.peers.Peers) {
		return &m.peers.Peers[m.cursor]
	}
	return nil
}

// truncate shortens a string to max length with ellipsis.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
