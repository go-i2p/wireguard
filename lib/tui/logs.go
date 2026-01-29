package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-i2p/wireguard/lib/rpc"
)

// LogsModel is the model for the logs view.
type LogsModel struct {
	logs     []rpc.LogEntry
	viewport viewport.Model
	ready    bool
	width    int
	height   int
	follow   bool // auto-scroll to bottom
}

// NewLogsModel creates a new logs view model.
func NewLogsModel() LogsModel {
	return LogsModel{
		follow: true,
	}
}

// SetData updates the logs data.
func (m *LogsModel) SetData(logs []rpc.LogEntry) {
	m.logs = logs
	if m.ready {
		m.updateViewport()
	}
}

// SetDimensions sets the view dimensions.
func (m *LogsModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
	if !m.ready {
		m.viewport = viewport.New(width, height-4)
		m.viewport.YPosition = 0
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = height - 4
	}
	m.updateViewport()
}

// Init initializes the logs model.
func (m LogsModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the logs view.
func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "f":
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
			}
			return m, nil
		case "g":
			m.viewport.GotoTop()
			m.follow = false
			return m, nil
		case "G":
			m.viewport.GotoBottom()
			m.follow = true
			return m, nil
		}
	}

	// Forward to viewport
	m.viewport, cmd = m.viewport.Update(msg)
	// If user scrolled, disable follow
	if m.viewport.AtBottom() {
		m.follow = true
	} else {
		m.follow = false
	}

	return m, cmd
}

// View renders the logs view.
func (m LogsModel) View() string {
	if !m.ready {
		return styles.Muted.Render("Initializing...")
	}

	if len(m.logs) == 0 {
		return styles.Muted.Render("No logs available")
	}

	// Header
	header := m.renderHeader()

	// Viewport with logs
	logView := m.viewport.View()

	// Footer with scroll info
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, logView, footer)
}

// renderHeader renders the logs header.
func (m LogsModel) renderHeader() string {
	followStatus := "OFF"
	if m.follow {
		followStatus = "ON"
	}
	return styles.Muted.Render(fmt.Sprintf(
		"Logs ─ %d entries │ Follow: %s │ (g)top (G)bottom (f)toggle follow",
		len(m.logs),
		followStatus,
	))
}

// renderFooter renders the scroll position footer.
func (m LogsModel) renderFooter() string {
	scrollPercent := m.viewport.ScrollPercent() * 100
	return styles.Muted.Render(fmt.Sprintf(
		"─── %.0f%% ───",
		scrollPercent,
	))
}

// updateViewport updates the viewport content.
func (m *LogsModel) updateViewport() {
	var content strings.Builder
	for _, entry := range m.logs {
		content.WriteString(m.formatLogEntry(entry))
		content.WriteString("\n")
	}
	m.viewport.SetContent(content.String())
	if m.follow {
		m.viewport.GotoBottom()
	}
}

// formatLogEntry formats a single log entry.
func (m LogsModel) formatLogEntry(entry rpc.LogEntry) string {
	timeStyle := styles.Muted
	levelStyle := m.levelStyle(entry.Level)
	msgStyle := lipgloss.NewStyle()

	timestamp := entry.Timestamp.Format("15:04:05")
	level := fmt.Sprintf("[%-5s]", entry.Level)

	return fmt.Sprintf("%s %s %s",
		timeStyle.Render(timestamp),
		levelStyle.Render(level),
		msgStyle.Render(entry.Message),
	)
}

// levelStyle returns the style for a log level.
func (m LogsModel) levelStyle(level string) lipgloss.Style {
	switch level {
	case "ERROR":
		return styles.Error
	case "WARN":
		return styles.Warning
	case "INFO":
		return styles.Success
	case "DEBUG":
		return styles.Muted
	default:
		return lipgloss.NewStyle()
	}
}
