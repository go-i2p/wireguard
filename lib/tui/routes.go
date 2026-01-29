package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-i2p/wireguard/lib/rpc"
)

// RoutesModel is the model for the routes view.
type RoutesModel struct {
	routes *rpc.RoutesListResult
	cursor int
	width  int
	height int
}

// NewRoutesModel creates a new routes view model.
func NewRoutesModel() RoutesModel {
	return RoutesModel{}
}

// SetData updates the routes data.
func (m *RoutesModel) SetData(routes *rpc.RoutesListResult) {
	m.routes = routes
	// Reset cursor if out of bounds
	if m.routes != nil && m.cursor >= len(m.routes.Routes) {
		m.cursor = max(0, len(m.routes.Routes)-1)
	}
}

// SetDimensions sets the view dimensions.
func (m *RoutesModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages for the routes view.
func (m RoutesModel) Update(msg tea.KeyMsg) (RoutesModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.routes != nil && m.cursor < len(m.routes.Routes)-1 {
			m.cursor++
		}
	}
	return m, nil
}

// View renders the routes view.
func (m RoutesModel) View() string {
	if m.routes == nil {
		return styles.Muted.Render("Loading routes...")
	}

	if len(m.routes.Routes) == 0 {
		return m.renderEmptyState()
	}

	var b strings.Builder

	// Table header
	header := fmt.Sprintf("%-16s %-20s %-6s %-16s %-20s", "TUNNEL IP", "NODE ID", "HOPS", "VIA", "LAST SEEN")
	b.WriteString(styles.TableHeader.Render(header))
	b.WriteString("\n")

	// Table rows
	for i, route := range m.routes.Routes {
		nodeID := truncate(route.NodeID, 20)
		via := route.ViaNodeID
		if via == "" {
			via = "(direct)"
		} else {
			via = truncate(via, 16)
		}

		hopStyle := styles.TableRow
		if route.HopCount == 0 {
			hopStyle = styles.Success
		} else if route.HopCount > 2 {
			hopStyle = styles.Warning
		}

		row := fmt.Sprintf("%-16s %-20s %-6s %-16s %-20s",
			route.TunnelIP,
			nodeID,
			hopStyle.Render(fmt.Sprintf("%d", route.HopCount)),
			via,
			route.LastSeen,
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
	directCount := 0
	for _, r := range m.routes.Routes {
		if r.HopCount == 0 {
			directCount++
		}
	}
	summary := fmt.Sprintf("Total: %d routes (%d direct, %d via relay)",
		m.routes.Total, directCount, m.routes.Total-directCount)
	b.WriteString(styles.Muted.Render(summary))

	return b.String()
}

// renderEmptyState renders the empty state for no routes.
func (m RoutesModel) renderEmptyState() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(2, 4).
		Width(50)

	content := lipgloss.JoinVertical(lipgloss.Center,
		styles.Bold.Render("No Routes"),
		"",
		styles.Muted.Render("Routes will appear when you"),
		styles.Muted.Render("connect to peers."),
	)

	return lipgloss.Place(
		m.width,
		m.height-2,
		lipgloss.Center,
		lipgloss.Center,
		box.Render(content),
	)
}

// SelectedRoute returns the currently selected route.
func (m RoutesModel) SelectedRoute() *rpc.RouteInfo {
	if m.routes == nil || len(m.routes.Routes) == 0 {
		return nil
	}
	if m.cursor >= 0 && m.cursor < len(m.routes.Routes) {
		return &m.routes.Routes[m.cursor]
	}
	return nil
}

// RenderRouteDetail renders detailed information about the selected route.
func (m RoutesModel) RenderRouteDetail() string {
	route := m.SelectedRoute()
	if route == nil {
		return styles.Muted.Render("No route selected")
	}

	var b strings.Builder

	// Title
	b.WriteString(styles.Bold.Render("Route Details"))
	b.WriteString("\n\n")

	// Route type indicator
	routeType := "Direct"
	if route.HopCount > 0 {
		routeType = fmt.Sprintf("Relayed (%d hops)", route.HopCount)
	}
	typeStyle := styles.Success
	if route.HopCount > 0 {
		typeStyle = styles.Warning
	}
	b.WriteString(fmt.Sprintf("  Type:        %s\n", typeStyle.Render(routeType)))

	// Core info
	b.WriteString(fmt.Sprintf("  Tunnel IP:   %s\n", route.TunnelIP))
	b.WriteString(fmt.Sprintf("  Node ID:     %s\n", route.NodeID))

	// Relay info
	if route.ViaNodeID != "" {
		b.WriteString(fmt.Sprintf("  Via Node:    %s\n", route.ViaNodeID))
	}

	// WireGuard key
	if route.WGPublicKey != "" {
		b.WriteString(fmt.Sprintf("  WG PubKey:   %s\n", truncate(route.WGPublicKey, 44)))
	}

	// I2P destination
	if route.I2PDest != "" {
		b.WriteString(fmt.Sprintf("  I2P Dest:    %s\n", route.I2PDest))
	}

	// Timestamps
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  Timestamps:\n"))
	if route.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("    Created:   %s\n", route.CreatedAt))
	}
	b.WriteString(fmt.Sprintf("    Last Seen: %s\n", route.LastSeen))

	return b.String()
}
