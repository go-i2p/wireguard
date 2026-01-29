package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles holds all the styles for the TUI.
var styles = struct {
	Title       lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	HelpText    lipgloss.Style
	StatusText  lipgloss.Style
	Error       lipgloss.Style
	Success     lipgloss.Style
	Warning     lipgloss.Style
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	Selected    lipgloss.Style
	Muted       lipgloss.Style
	Bold        lipgloss.Style
	Input       lipgloss.Style
	InputFocus  lipgloss.Style
	Box         lipgloss.Style
	BoxTitle    lipgloss.Style
}{
	Title: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(0, 1),

	TabActive: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("236")).
		Padding(0, 2),

	TabInactive: lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Padding(0, 2),

	HelpText: lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")),

	StatusText: lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")),

	Error: lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true),

	Success: lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true),

	Warning: lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")),

	TableHeader: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")),

	TableRow: lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")),

	Selected: lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Bold(true),

	Muted: lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")),

	Bold: lipgloss.NewStyle().
		Bold(true),

	Input: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1),

	InputFocus: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(0, 1),

	Box: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2),

	BoxTitle: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")),
}

// PeerStateStyle returns the style for a peer state.
func PeerStateStyle(state string) lipgloss.Style {
	switch state {
	case "Connected":
		return styles.Success
	case "Pending":
		return styles.Warning
	case "Disconnected", "Rejected":
		return styles.Error
	default:
		return styles.Muted
	}
}
