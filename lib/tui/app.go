// Package tui provides an interactive terminal user interface for i2plan.
// It uses BubbleTea for the application framework and communicates with
// the running node via RPC.
package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
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
	numTabs // Total number of tabs (must be last)
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
	default:
		return "Unknown"
	}
}

// Model is the main TUI application model.
type Model struct {
	// RPC client
	client *rpc.Client

	// RPC connection state
	rpcConfig     Config
	rpcConnected  bool
	rpcRetryCount int
	rpcMaxRetries int

	// Current state
	activeTab      Tab
	width          int
	height         int
	ready          bool
	err            error
	errTimestamp   time.Time
	errDisplayTime time.Duration
	lastRefresh    time.Time
	showHelp       bool

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
		client:        client,
		rpcConfig:     cfg,
		rpcConnected:  true,
		rpcMaxRetries: 3,
		activeTab:     TabStatus,
		spinner:       s,
		peersView:     NewPeersModel(),
		routesView:    NewRoutesModel(),
		invitesView:   NewInvitesModel(),
		statusView:    NewStatusModel(),
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
		return m.handleKeyMsg(msg, cmds)
	case tea.WindowSizeMsg:
		m.handleWindowSizeMsg(msg)
	case refreshMsg:
		cmds = m.handleRefreshMsg(msg, cmds)
	case tickMsg:
		cmds = append(cmds, m.refreshData)
	case cleanupMsg:
		// Close RPC client before exit
		if err := m.Close(); err != nil {
			log.WithError(err).Warn("failed to close RPC client")
		}
		return m, nil
	case reconnectMsg:
		cmds = append(cmds, m.attemptReconnection())
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
		// Update invites spinner too
		var inviteCmd tea.Cmd
		m.invitesView, inviteCmd = m.invitesView.UpdateSpinner(msg)
		cmds = append(cmds, inviteCmd)
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

// handleKeyMsg processes keyboard input messages and returns the updated model and commands.
func (m Model) handleKeyMsg(msg tea.KeyMsg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if handled, cmd := m.handleGlobalKeys(msg); handled {
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	cmd := m.delegateToActiveView(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleGlobalKeys processes global keyboard shortcuts and returns whether the key was handled.
func (m *Model) handleGlobalKeys(msg tea.KeyMsg) (handled bool, cmd tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		// Send cleanup command before quitting
		return true, tea.Sequence(
			func() tea.Msg { return cleanupMsg{} },
			tea.Quit,
		)
	case key.Matches(msg, keys.Tab):
		m.activeTab = Tab((int(m.activeTab) + 1) % int(numTabs))
		return true, nil
	case key.Matches(msg, keys.ShiftTab):
		m.activeTab = Tab((int(m.activeTab) + int(numTabs) - 1) % int(numTabs))
		return true, nil
	case key.Matches(msg, keys.Refresh):
		return true, m.refreshData
	case key.Matches(msg, keys.Peers):
		m.activeTab = TabPeers
		return true, nil
	case key.Matches(msg, keys.Routes):
		m.activeTab = TabRoutes
		return true, nil
	case key.Matches(msg, keys.Invites):
		m.activeTab = TabInvites
		return true, nil
	case key.Matches(msg, keys.Status):
		m.activeTab = TabStatus
		return true, nil
	case key.Matches(msg, keys.Help):
		m.showHelp = !m.showHelp
		return true, nil
	}
	return false, nil
}

// delegateToActiveView passes the key message to the currently active view.
func (m *Model) delegateToActiveView(msg tea.KeyMsg) tea.Cmd {
	switch m.activeTab {
	case TabPeers:
		var cmd tea.Cmd
		m.peersView, cmd = m.peersView.Update(msg, m.client)
		return cmd
	case TabRoutes:
		var cmd tea.Cmd
		m.routesView, cmd = m.routesView.Update(msg)
		return cmd
	case TabInvites:
		var cmd tea.Cmd
		m.invitesView, cmd = m.invitesView.Update(msg, m.client)
		return cmd
	}
	return nil
}

// handleWindowSizeMsg updates the model dimensions when the window is resized.
func (m *Model) handleWindowSizeMsg(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true
	contentHeight := m.height - 4 // Header + footer
	m.peersView.SetDimensions(m.width, contentHeight)
	m.routesView.SetDimensions(m.width, contentHeight)
	m.invitesView.SetDimensions(m.width, contentHeight)
	m.statusView.SetDimensions(m.width, contentHeight)
}

// handleRefreshMsg updates the model with fresh data from RPC.
func (m *Model) handleRefreshMsg(msg refreshMsg, cmds []tea.Cmd) []tea.Cmd {
	m.status = msg.status
	m.peers = msg.peers
	m.routes = msg.routes
	if msg.err != nil {
		m.err = msg.err
		m.errTimestamp = time.Now()
	} else {
		m.err = nil // Clear error on success
	}
	m.lastRefresh = time.Now()
	m.peersView.SetData(msg.peers)
	m.routesView.SetData(msg.routes)
	m.statusView.SetData(msg.status)
	return append(cmds, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	}))
}

// View renders the TUI.
func (m Model) View() string {
	if !m.ready {
		return fmt.Sprintf("%s Loading...", m.spinner.View())
	}

	// Show help overlay if requested
	if m.showHelp {
		return m.renderHelpOverlay()
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
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

// renderHeader renders the tab bar.
func (m Model) renderHeader() string {
	tabs := []Tab{TabPeers, TabRoutes, TabInvites, TabStatus}

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
	}

	// Global help
	helpItems = append(helpItems, "tab switch", "r refresh", "q quit")

	help := strings.Join(helpItems, " • ")

	// Status info
	var statusInfo string
	if m.status != nil {
		statusInfo = fmt.Sprintf("Peers: %d | %s", m.status.PeerCount, m.status.Uptime)
	}
	// Display error with auto-clear after configured duration
	if m.err != nil {
		if time.Since(m.errTimestamp) > m.errDisplayTime {
			m.err = nil // Auto-clear old errors
		} else {
			statusInfo = styles.Error.Render(m.err.Error())
		}
	}

	footer := lipgloss.JoinHorizontal(
		lipgloss.Top,
		styles.HelpText.Render(help),
		strings.Repeat(" ", max(0, m.width-lipgloss.Width(help)-lipgloss.Width(statusInfo)-2)),
		styles.StatusText.Render(statusInfo),
	)

	return footer
}

// renderHelpOverlay renders the help screen overlay.
func (m Model) renderHelpOverlay() string {
	helpBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(currentTheme.Primary).
		Padding(2, 4).
		Width(70)

	help := lipgloss.JoinVertical(lipgloss.Left,
		styles.BoxTitle.Render("Keyboard Shortcuts"),
		"",
		styles.Bold.Render("Global:"),
		"  ?          Show/hide this help",
		"  tab        Next tab",
		"  shift+tab  Previous tab",
		"  1-4        Jump to tab (Peers/Routes/Invites/Status)",
		"  r          Refresh data",
		"  q          Quit",
		"",
		styles.Bold.Render("Peers:"),
		"  ↑/k        Move up",
		"  ↓/j        Move down",
		"  c          Connect to selected peer",
		"",
		styles.Bold.Render("Routes:"),
		"  ↑/k        Move up",
		"  ↓/j        Move down",
		"",
		styles.Bold.Render("Invites:"),
		"  n          Create new invite",
		"  a          Accept invite code",
		"  enter      Confirm action",
		"  esc        Cancel action",
		"",
		styles.HelpText.Render("Press ? or Esc to close"),
	)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		helpBox.Render(help),
	)
}

// refreshData fetches fresh data from RPC with automatic reconnection on connection errors.
func (m Model) refreshData() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var msg refreshMsg

	status, err := m.client.Status(ctx)
	if err != nil {
		log.WithError(err).Debug("RPC call failed")
		msg.err = err

		// Attempt reconnection if this is a connection error
		if isConnectionError(err) && m.rpcRetryCount < m.rpcMaxRetries {
			m.rpcRetryCount++
			log.WithField("retry", m.rpcRetryCount).Debug("triggering RPC reconnection")
			return reconnectMsg{err: err}
		}

		m.rpcConnected = false
		return msg
	}

	// Reset retry count on successful connection
	m.rpcRetryCount = 0
	m.rpcConnected = true
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

type cleanupMsg struct{}

type reconnectMsg struct {
	err error
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// attemptReconnection tries to reconnect to the RPC server.
func (m *Model) attemptReconnection() tea.Cmd {
	return func() tea.Msg {
		log.Info("attempting RPC reconnection")

		// Close existing connection
		if m.client != nil {
			m.client.Close()
		}

		// Wait a bit before reconnecting
		time.Sleep(time.Second)

		// Attempt to create new connection
		client, err := rpc.NewClient(rpc.ClientConfig{
			UnixSocketPath: m.rpcConfig.RPCSocketPath,
			AuthFile:       m.rpcConfig.RPCAuthFile,
		})
		if err != nil {
			log.WithError(err).Warn("reconnection failed")
			return reconnectMsg{err: err}
		}

		log.Info("reconnection successful")
		m.client = client
		m.rpcConnected = true
		m.rpcRetryCount = 0

		// Trigger immediate refresh with new connection
		return tickMsg(time.Now())
	}
}

// isConnectionError checks if an error is related to connection failure.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Check for common connection error patterns
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "broken pipe") ||
		strings.Contains(err.Error(), "connection reset")
}
