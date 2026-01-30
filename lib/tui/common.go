package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// renderEmptyState creates a centered empty state view with a title, subtitle, and optional help text.
// This helper function standardizes empty state rendering across all views.
func renderEmptyState(width, height int, title, subtitle string, helpText []string) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(2, 4).
		Width(50)

	lines := []string{
		styles.Bold.Render(title),
		"",
	}

	if subtitle != "" {
		lines = append(lines, styles.Muted.Render(subtitle))
	}

	if len(helpText) > 0 {
		lines = append(lines, "")
		for _, help := range helpText {
			lines = append(lines, styles.HelpText.Render(help))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Center, lines...)
	return lipgloss.Place(width, height-2, lipgloss.Center, lipgloss.Center, box.Render(content))
}
