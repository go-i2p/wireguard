// Package tui provides an interactive terminal user interface for i2plan.
// It uses BubbleTea for the application framework and communicates with
// the running node via RPC.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-i2p/wireguard/lib/rpc"
)

// Tab represents a UI tab.
type Tab int

const (
	TabPeers Tab = iota
	TabRoutes
	TabInvites
	TabStatus
	TabLogs
)

func (t Tab) String() string {
	switch t {
	case TabPeers:
		return "Peers"
	case TabRoutes:
		return "Routes"
	case TabInvites:
		return "Invites"
	case TabStatus:
		return "Status"
	case TabLogs:
		return "Logs"
	default:
		return "Unknown"
	}
}

// Model is the main TUI application model.
type Model struct {
	// RPC client
	client *rpc.Client

	// Current state
	activeTab   Tab
	width       int
	height      int
	ready       bool
	err         error
	lastRefresh time.Time

	// Data from RPC
	status *rpc.StatusResult
	peers  *rpc.PeersListResult
	routes *rpc.RoutesListResult

	// Sub-models
	spinner     spinner.Model
	peersView   PeersModel
	routesView  RoutesModel
	invitesView InvitesModel
	statusView  StatusModel
	logsView    LogsModel

	// Input mode
	inputMode bool
}

// Config holds TUI configuration.
type Config struct {
	// RPCSocketPath is the path to the RPC Unix socket.
	RPCSocketPath string
	// RPCAuthFile is the path to the RPC auth token file.
	RPCAuthFile string
	// RefreshInterval is how often to refresh data.
	RefreshInterval time.Duration
}

// New creates a new TUI model.
func New(cfg Config) (*Model, error) {
	client, err := rpc.NewClient(rpc.ClientConfig{
		UnixSocketPath: cfg.RPCSocketPath,
		AuthFile:       cfg.RPCAuthFile,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to RPC: %w", err)
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return &Model{
		client:      client,
		activeTab:   TabStatus,
		spinner:     s,
		peersView:   NewPeersModel(),
		routesView:  NewRoutesModel(),
		invitesView: NewInvitesModel(),
		statusView:  NewStatusModel(),
		logsView:    NewLogsModel(),
	}, nil
}

// Init initializes the TUI model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.refreshData,
		tea.SetWindowTitle("i2plan"),
	)
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global keys
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Tab):
			m.activeTab = Tab((int(m.activeTab) + 1) % 5)
		case key.Matches(msg, keys.ShiftTab):
			m.activeTab = Tab((int(m.activeTab) + 4) % 5)
		case key.Matches(msg, keys.Refresh):
			cmds = append(cmds, m.refreshData)
		case key.Matches(msg, keys.Peers):
			m.activeTab = TabPeers
		case key.Matches(msg, keys.Routes):
			m.activeTab = TabRoutes
		case key.Matches(msg, keys.Invites):
			m.activeTab = TabInvites
		case key.Matches(msg, keys.Status):
			m.activeTab = TabStatus
		case key.Matches(msg, keys.Logs):
			m.activeTab = TabLogs
		}

		// Pass to active view
		switch m.activeTab {
		case TabPeers:
			var cmd tea.Cmd
			m.peersView, cmd = m.peersView.Update(msg, m.client)
			cmds = append(cmds, cmd)
		case TabRoutes:
			var cmd tea.Cmd
			m.routesView, cmd = m.routesView.Update(msg)
			cmds = append(cmds, cmd)
		case TabInvites:
			var cmd tea.Cmd
			m.invitesView, cmd = m.invitesView.Update(msg, m.client)
			cmds = append(cmds, cmd)
		case TabLogs:
			var cmd tea.Cmd
			m.logsView, cmd = m.logsView.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Update view dimensions
		contentHeight := m.height - 4 // Header + footer
		m.peersView.SetDimensions(m.width, contentHeight)
		m.routesView.SetDimensions(m.width, contentHeight)
		m.invitesView.SetDimensions(m.width, contentHeight)
		m.statusView.SetDimensions(m.width, contentHeight)
		m.logsView.SetDimensions(m.width, contentHeight)

	case refreshMsg:
		m.status = msg.status
		m.peers = msg.peers
		m.routes = msg.routes
		m.err = msg.err
		m.lastRefresh = time.Now()
		// Update views with new data
		m.peersView.SetData(msg.peers)
		m.routesView.SetData(msg.routes)
		m.statusView.SetData(msg.status)
		// Schedule next refresh
		cmds = append(cmds, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}))

	case tickMsg:
		cmds = append(cmds, m.refreshData)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case inviteCreatedMsg:
		m.invitesView.SetCreatedInvite(msg.invite)

	case inviteAcceptedMsg:
		m.invitesView.SetAcceptResult(msg.result)
		cmds = append(cmds, m.refreshData)

	case errMsg:
		m.err = msg.err
	}

	return m, tea.Batch(cmds...)
}

// View renders the TUI.
func (m Model) View() string {
	if !m.ready {
		return fmt.Sprintf("%s Loading...", m.spinner.View())
	}

	var b strings.Builder

	// Header with tabs
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Content area
	switch m.activeTab {
	case TabPeers:
		b.WriteString(m.peersView.View())
	case TabRoutes:
		b.WriteString(m.routesView.View())
	case TabInvites:
		b.WriteString(m.invitesView.View())
	case TabStatus:
		b.WriteString(m.statusView.View())
	case TabLogs:
		b.WriteString(m.logsView.View())
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

// renderHeader renders the tab bar.
func (m Model) renderHeader() string {
	tabs := []Tab{TabPeers, TabRoutes, TabInvites, TabStatus, TabLogs}

	var renderedTabs []string
	for _, tab := range tabs {
		style := styles.TabInactive
		if tab == m.activeTab {
			style = styles.TabActive
		}
		renderedTabs = append(renderedTabs, style.Render(tab.String()))
	}

	title := styles.Title.Render("i2plan")
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	return lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", tabBar)
}

// renderFooter renders the help text.
func (m Model) renderFooter() string {
	var helpItems []string

	// Tab-specific help
	switch m.activeTab {
	case TabPeers:
		helpItems = append(helpItems, "↑↓ navigate", "c connect")
	case TabInvites:
		helpItems = append(helpItems, "n new invite", "a accept invite")
	case TabLogs:
		helpItems = append(helpItems, "↑↓ scroll")
	}

	// Global help
	helpItems = append(helpItems, "tab switch", "r refresh", "q quit")

	help := strings.Join(helpItems, " • ")

	// Status info
	var statusInfo string
	if m.status != nil {
		statusInfo = fmt.Sprintf("Peers: %d | %s", m.status.PeerCount, m.status.Uptime)
	}
	if m.err != nil {
		statusInfo = styles.Error.Render(m.err.Error())
	}

	footer := lipgloss.JoinHorizontal(
		lipgloss.Top,
		styles.HelpText.Render(help),
		strings.Repeat(" ", max(0, m.width-lipgloss.Width(help)-lipgloss.Width(statusInfo)-2)),
		styles.StatusText.Render(statusInfo),
	)

	return footer
}

// refreshData fetches fresh data from RPC.
func (m Model) refreshData() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var msg refreshMsg

	status, err := m.client.Status(ctx)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.status = status

	peers, err := m.client.PeersList(ctx)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.peers = peers

	routes, err := m.client.RoutesList(ctx)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.routes = routes

	return msg
}

// Close cleans up resources.
func (m *Model) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// Messages

type refreshMsg struct {
	status *rpc.StatusResult
	peers  *rpc.PeersListResult
	routes *rpc.RoutesListResult
	err    error
}

type tickMsg time.Time

type inviteCreatedMsg struct {
	invite *rpc.InviteCreateResult
}

type inviteAcceptedMsg struct {
	result *rpc.InviteAcceptResult
}

type errMsg struct {
	err error
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
