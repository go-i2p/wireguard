package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-i2p/wireguard/lib/rpc"
)

// StatusModel is the model for the status view.
type StatusModel struct {
	status *rpc.StatusResult
	width  int
	height int
}

// NewStatusModel creates a new status view model.
func NewStatusModel() StatusModel {
	return StatusModel{}
}

// SetData updates the status data.
func (m *StatusModel) SetData(status *rpc.StatusResult) {
	m.status = status
}

// SetDimensions sets the view dimensions.
func (m *StatusModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// View renders the status view.
func (m StatusModel) View() string {
	if m.status == nil {
		return styles.Muted.Render("Loading status...")
	}

	var b strings.Builder
	b.WriteString(m.renderNodeStatusBox())
	b.WriteString("\n\n")
	b.WriteString(m.renderNetworkBox())
	return b.String()
}

// renderNodeStatusBox creates the main node status display box.
func (m StatusModel) renderNodeStatusBox() string {
	mainBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(60)

	stateStyle := styles.Success
	if m.status.State != "running" {
		stateStyle = styles.Warning
	}

	mainContent := lipgloss.JoinVertical(lipgloss.Left,
		styles.BoxTitle.Render("Node Status"),
		"",
		m.statusRow("State", stateStyle.Render(m.status.State)),
		m.statusRow("Name", m.status.NodeName),
		m.statusRow("Node ID", truncate(m.status.NodeID, 30)),
		m.statusRow("Version", m.status.Version),
		m.statusRow("Uptime", m.status.Uptime),
	)

	return mainBox.Render(mainContent)
}

// renderNetworkBox creates the network information display box.
func (m StatusModel) renderNetworkBox() string {
	networkBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(60)

	peerCountStyle := styles.Muted
	if m.status.PeerCount > 0 {
		peerCountStyle = styles.Success
	}

	networkContent := lipgloss.JoinVertical(lipgloss.Left,
		styles.BoxTitle.Render("Network"),
		"",
		m.statusRow("Tunnel IP", m.formatOptional(m.status.TunnelIP)),
		m.statusRow("I2P Dest", m.formatOptional(m.status.I2PDestination)),
		m.statusRow("Peers", peerCountStyle.Render(fmt.Sprintf("%d", m.status.PeerCount))),
	)

	return networkBox.Render(networkContent)
}

// statusRow formats a status row with label and value.
func (m StatusModel) statusRow(label, value string) string {
	labelStyle := styles.Muted.Width(15)
	return labelStyle.Render(label+":") + " " + value
}

// formatOptional formats an optional value.
func (m StatusModel) formatOptional(value string) string {
	if value == "" {
		return styles.Muted.Render("(not set)")
	}
	return value
}
