package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// ColorScheme defines a theme's color palette.
type ColorScheme struct {
	Primary lipgloss.Color
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
	Muted   lipgloss.Color
	Border  lipgloss.Color
}

// DefaultColorScheme is the default pink/cyan theme.
var DefaultColorScheme = ColorScheme{
	Primary: lipgloss.Color("205"),
	Success: lipgloss.Color("82"),
	Warning: lipgloss.Color("214"),
	Error:   lipgloss.Color("196"),
	Muted:   lipgloss.Color("241"),
	Border:  lipgloss.Color("240"),
}

// currentTheme is the active color scheme (can be changed at runtime).
var currentTheme = DefaultColorScheme

// SetColorScheme changes the active color scheme.
func SetColorScheme(scheme ColorScheme) {
	currentTheme = scheme
	// Rebuild styles with new colors
	initStyles()
}

// Styles holds all the styles for the TUI.
var styles struct {
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
}

func init() {
	initStyles()
}

// initStyles initializes or updates all styles based on currentTheme.
func initStyles() {
	styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(currentTheme.Primary).
		Padding(0, 1)

	styles.TabActive = lipgloss.NewStyle().
		Bold(true).
		Foreground(currentTheme.Primary).
		Background(lipgloss.Color("236")).
		Padding(0, 2)

	styles.TabInactive = lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Padding(0, 2)

	styles.HelpText = lipgloss.NewStyle().
		Foreground(currentTheme.Muted)

	styles.StatusText = lipgloss.NewStyle().
		Foreground(lipgloss.Color("244"))

	styles.Error = lipgloss.NewStyle().
		Foreground(currentTheme.Error).
		Bold(true)

	styles.Success = lipgloss.NewStyle().
		Foreground(currentTheme.Success).
		Bold(true)

	styles.Warning = lipgloss.NewStyle().
		Foreground(currentTheme.Warning)

	styles.TableHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(currentTheme.Border)

	styles.TableRow = lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	styles.Selected = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Bold(true)

	styles.Muted = lipgloss.NewStyle().
		Foreground(currentTheme.Muted)

	styles.Bold = lipgloss.NewStyle().
		Bold(true)

	styles.Input = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(currentTheme.Border).
		Padding(0, 1)

	styles.InputFocus = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(currentTheme.Primary).
		Padding(0, 1)

	styles.Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(currentTheme.Border).
		Padding(1, 2)

	styles.BoxTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(currentTheme.Primary)
}

// PeerStateStyle returns the appropriate style for displaying a peer's connection state.
// Connected peers are shown in green (Success), Pending in yellow (Warning),
// Disconnected/Rejected in red (Error), and unknown states in gray (Muted).
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
