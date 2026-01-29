package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// SetData updates the peers data.
func (m *PeersModel) SetData(peers *rpc.PeersListResult) {
	m.peers = peers
	// Reset cursor if out of bounds
	if m.peers != nil && m.cursor >= len(m.peers.Peers) {
		m.cursor = max(0, len(m.peers.Peers)-1)
	}
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
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(2, 4).
		Width(50)

	content := lipgloss.JoinVertical(lipgloss.Center,
		styles.Bold.Render("No Peers Connected"),
		"",
		styles.Muted.Render("Generate an invite code and share it"),
		styles.Muted.Render("with someone to connect."),
		"",
		styles.HelpText.Render("Press Tab → Invites → n to create invite"),
	)

	return lipgloss.Place(
		m.width,
		m.height-2,
		lipgloss.Center,
		lipgloss.Center,
		box.Render(content),
	)
}

// SelectedPeer returns the currently selected peer.
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
