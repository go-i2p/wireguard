package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-i2p/wireguard/lib/rpc"
	qrterminal "github.com/mdp/qrterminal/v3"
)

// InvitesMode represents the current mode of the invites view.
type InvitesMode int

const (
	InvitesModeNormal InvitesMode = iota
	InvitesModeCreate
	InvitesModeAccept
	InvitesModeScan
)

// Timeout constants for invite operations.
const (
	inviteCreateTimeout = 10 * time.Second
	inviteAcceptTimeout = 30 * time.Second
)

// InvitesModel is the model for the invites view.
type InvitesModel struct {
	mode          InvitesMode
	width         int
	height        int
	textInput     textinput.Model
	spinner       spinner.Model
	createdInvite *rpc.InviteCreateResult
	acceptResult  *rpc.InviteAcceptResult
	error         string
	message       string
	qrCode        string // Stores rendered QR code for display
	pendingCreate bool   // Tracks if invite creation is in progress
	pendingAccept bool   // Tracks if invite acceptance is in progress
}

// NewInvitesModel creates a new invites view model.
func NewInvitesModel() InvitesModel {
	ti := textinput.New()
	ti.Placeholder = "Paste invite code here..."
	ti.CharLimit = 2000
	ti.Width = 60

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(currentTheme.Primary)

	return InvitesModel{
		textInput: ti,
		spinner:   s,
	}
}

// SetDimensions sets the view dimensions.
func (m *InvitesModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// SetCreatedInvite sets the created invite result and clears any pending state.
// This method ignores stale messages by checking the pendingCreate flag to prevent
// race conditions when users cancel operations.
func (m *InvitesModel) SetCreatedInvite(invite *rpc.InviteCreateResult) {
	// Only update if we were actually waiting for this result
	if !m.pendingCreate {
		return
	}
	m.pendingCreate = false
	m.createdInvite = invite
	m.error = ""

	// Generate QR code for terminal display
	if invite != nil && invite.InviteCode != "" {
		m.qrCode = m.generateQRCode(invite.InviteCode)
	}
}

// SetAcceptResult sets the accept result and resets the mode to normal.
// This method ignores stale messages by checking the pendingAccept flag to prevent
// race conditions when users cancel operations.
func (m *InvitesModel) SetAcceptResult(result *rpc.InviteAcceptResult) {
	// Only update if we were actually waiting for this result
	if !m.pendingAccept {
		return
	}
	m.pendingAccept = false
	m.acceptResult = result
	m.error = ""
	m.mode = InvitesModeNormal
}

// Update handles keyboard input messages for the invites view.
// The behavior changes based on the current mode (Normal, Create, Accept, or Scan).
func (m InvitesModel) Update(msg tea.KeyMsg, client *rpc.Client) (InvitesModel, tea.Cmd) {
	switch m.mode {
	case InvitesModeNormal:
		return m.handleNormalModeKey(msg, client)
	case InvitesModeCreate:
		return m.handleCreateModeKey(msg)
	case InvitesModeAccept:
		return m.handleAcceptModeKey(msg, client)
	case InvitesModeScan:
		return m.handleScanModeKey(msg, client)
	}
	return m, nil
}

// UpdateSpinner updates the spinner animation (called from app.go).
// This must be called on every spinner.TickMsg to keep the animation running.
func (m InvitesModel) UpdateSpinner(msg tea.Msg) (InvitesModel, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// handleNormalModeKey processes key input in normal mode.
func (m InvitesModel) handleNormalModeKey(msg tea.KeyMsg, client *rpc.Client) (InvitesModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.NewInvite):
		// Ignore if already creating an invite
		if m.pendingCreate {
			return m, nil
		}
		m.mode = InvitesModeCreate
		m.pendingCreate = true
		m.createdInvite = nil
		m.error = ""
		m.message = ""
		return m, m.createInvite(client)
	case key.Matches(msg, keys.Accept):
		m.mode = InvitesModeAccept
		m.textInput.Focus()
		m.textInput.SetValue("")
		m.error = ""
		m.message = ""
		return m, textinput.Blink
	case key.Matches(msg, keys.Scan):
		m.mode = InvitesModeScan
		m.textInput.Focus()
		m.textInput.SetValue("")
		m.error = ""
		m.message = ""
		return m, textinput.Blink
	}
	return m, nil
}

// handleCreateModeKey processes key input in create mode.
func (m InvitesModel) handleCreateModeKey(msg tea.KeyMsg) (InvitesModel, tea.Cmd) {
	if key.Matches(msg, keys.Escape) || key.Matches(msg, keys.Enter) {
		m.mode = InvitesModeNormal
		// Clear pending flag if user cancels
		if key.Matches(msg, keys.Escape) {
			m.pendingCreate = false
		}
	}
	return m, nil
}

// handleAcceptModeKey processes key input in accept mode.
func (m InvitesModel) handleAcceptModeKey(msg tea.KeyMsg, client *rpc.Client) (InvitesModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.mode = InvitesModeNormal
		m.textInput.Blur()
		// Clear pending flag if user cancels
		m.pendingAccept = false
	case key.Matches(msg, keys.Enter):
		code := strings.TrimSpace(m.textInput.Value())
		if code == "" {
			return m, nil
		}

		// Validate invite code format (Issue #10)
		if !strings.HasPrefix(code, "i2plan://") {
			m.error = "Invalid invite format (must start with i2plan://)"
			return m, nil
		}

		if len(code) < 20 {
			m.error = "Invite code too short"
			return m, nil
		}

		// Ignore if already accepting an invite
		if m.pendingAccept {
			return m, nil
		}

		m.pendingAccept = true
		m.textInput.Blur()
		m.error = ""
		return m, m.acceptInvite(client, code)
	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the invites view.
func (m InvitesModel) View() string {
	switch m.mode {
	case InvitesModeCreate:
		return m.viewCreate()
	case InvitesModeAccept:
		return m.viewAccept()
	case InvitesModeScan:
		return m.viewScan()
	default:
		return m.viewNormal()
	}
}

// viewNormal renders the normal invites view.
func (m InvitesModel) viewNormal() string {
	var b strings.Builder

	b.WriteString(m.renderInstructionsBox())
	b.WriteString("\n\n")

	if m.createdInvite != nil {
		b.WriteString(m.renderCreatedInviteBox())
		b.WriteString("\n\n")
	}

	if m.acceptResult != nil {
		b.WriteString(m.renderAcceptResultBox())
	}

	if m.error != "" {
		b.WriteString("\n")
		b.WriteString(styles.Error.Render(fmt.Sprintf("⚠ %s", m.error)))
	}

	return b.String()
}

// renderInstructionsBox creates the instructions box for the invites view.
func (m InvitesModel) renderInstructionsBox() string {
	instructionsBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(60)

	instructions := lipgloss.JoinVertical(lipgloss.Left,
		styles.BoxTitle.Render("Invite System"),
		"",
		"Invites allow new nodes to join your mesh network.",
		"",
		styles.Bold.Render("n")+" - Generate a new invite code to share",
		styles.Bold.Render("a")+" - Accept an invite code from someone else",
		styles.Bold.Render("s")+" - Scan a QR code (paste from image)",
	)

	return instructionsBox.Render(instructions)
}

// renderCreatedInviteBox creates the box displaying a newly created invite.
func (m InvitesModel) renderCreatedInviteBox() string {
	inviteBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("82")).
		Padding(1, 2).
		Width(m.width - 4)

	content := lipgloss.JoinVertical(lipgloss.Left,
		styles.Success.Render("✓ Invite Created"),
		"",
		styles.Muted.Render("Code:"),
		m.createdInvite.InviteCode,
		"",
		styles.Muted.Render("Expires: "+m.createdInvite.ExpiresAt),
	)

	// Add QR code if available
	if m.qrCode != "" {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			styles.Muted.Render("QR Code:"),
			m.qrCode,
		)
	}

	return inviteBox.Render(content)
}

// renderAcceptResultBox creates the box displaying the result of accepting an invite.
func (m InvitesModel) renderAcceptResultBox() string {
	resultBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("82")).
		Padding(1, 2).
		Width(60)

	content := lipgloss.JoinVertical(lipgloss.Left,
		styles.Success.Render("✓ Joined Network"),
		"",
		"Network: "+m.acceptResult.NetworkID,
		"Peer: "+truncate(m.acceptResult.PeerNodeID, 20),
		"Your IP: "+m.acceptResult.TunnelIP,
	)

	return resultBox.Render(content)
}

// viewCreate renders the create invite view.
func (m InvitesModel) viewCreate() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(2, 4).
		Width(m.width - 10)

	content := m.buildCreateInviteContent()

	return lipgloss.Place(
		m.width,
		m.height-2,
		lipgloss.Center,
		lipgloss.Center,
		box.Render(content),
	)
}

// buildCreateInviteContent builds the content for the create invite modal.
func (m InvitesModel) buildCreateInviteContent() string {
	if m.createdInvite != nil {
		return m.buildInviteSuccessContent()
	}
	if m.error != "" {
		return m.buildInviteErrorContent()
	}
	return m.buildInviteLoadingContent()
}

// buildInviteSuccessContent creates the content when invite is successfully created.
func (m InvitesModel) buildInviteSuccessContent() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.BoxTitle.Render("Invite Code Generated"),
		"",
		"Share this code with the person you want to invite:",
		"",
		m.createdInvite.InviteCode,
		"",
		styles.Muted.Render("Expires: "+m.createdInvite.ExpiresAt),
		styles.Muted.Render("Max uses: 1"),
		"",
		styles.HelpText.Render("Press Enter or Esc to close"),
	)
}

// buildInviteErrorContent creates the content when invite creation fails.
func (m InvitesModel) buildInviteErrorContent() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.BoxTitle.Render("Create Invite"),
		"",
		styles.Error.Render(fmt.Sprintf("⚠ %s", m.error)),
		"",
		styles.HelpText.Render("Press Esc to close"),
	)
}

// buildInviteLoadingContent creates the content while invite is being created.
func (m InvitesModel) buildInviteLoadingContent() string {
	return lipgloss.JoinVertical(lipgloss.Center,
		styles.BoxTitle.Render("Creating Invite..."),
		"",
		m.spinner.View()+" Generating invite code...",
	)
}

// viewAccept renders the accept invite view.
func (m InvitesModel) viewAccept() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(2, 4).
		Width(70)

	var content string
	if m.pendingAccept {
		content = lipgloss.JoinVertical(lipgloss.Center,
			styles.BoxTitle.Render("Accepting Invite..."),
			"",
			m.spinner.View()+" Joining network...",
		)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left,
			styles.BoxTitle.Render("Accept Invite"),
			"",
			"Paste the invite code you received:",
			"",
			m.textInput.View(),
			"",
		)

		if m.error != "" {
			content = lipgloss.JoinVertical(lipgloss.Left,
				content,
				styles.Error.Render(fmt.Sprintf("⚠ %s", m.error)),
				"",
			)
		}

		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			styles.HelpText.Render("Enter to accept • Esc to cancel"),
		)
	}

	return lipgloss.Place(
		m.width,
		m.height-2,
		lipgloss.Center,
		lipgloss.Center,
		box.Render(content),
	)
}

// createInvite creates a new invite via RPC.
func (m InvitesModel) createInvite(client *rpc.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), inviteCreateTimeout)
		defer cancel()

		result, err := client.InviteCreate(ctx, "24h", 1)
		if err != nil {
			return errMsg{err: err}
		}
		return inviteCreatedMsg{invite: result}
	}
}

// acceptInvite accepts an invite via RPC.
func (m InvitesModel) acceptInvite(client *rpc.Client, code string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), inviteAcceptTimeout)
		defer cancel()

		result, err := client.InviteAccept(ctx, code)
		if err != nil {
			return errMsg{err: err}
		}
		return inviteAcceptedMsg{result: result}
	}
}

// generateQRCode generates a terminal-rendered QR code for the invite code.
func (m InvitesModel) generateQRCode(inviteCode string) string {
	// Create a string builder to capture the QR code output
	var qrBuilder strings.Builder

	// Configure QR terminal with medium level error correction
	config := qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    &qrBuilder,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 1,
	}

	// Generate QR code
	qrterminal.GenerateWithConfig(inviteCode, config)

	return qrBuilder.String()
}

// handleScanModeKey processes key input in scan mode.
// Note: Terminal webcam scanning is not feasible, so this mode allows
// pasting QR code data extracted from an image file using external tools.
func (m InvitesModel) handleScanModeKey(msg tea.KeyMsg, client *rpc.Client) (InvitesModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.mode = InvitesModeNormal
		m.textInput.Blur()
	case key.Matches(msg, keys.Enter):
		code := strings.TrimSpace(m.textInput.Value())
		if code == "" {
			return m, nil
		}

		// Validate invite code format
		if !strings.HasPrefix(code, "i2plan://") {
			m.error = "Invalid invite format (must start with i2plan://)"
			return m, nil
		}

		if len(code) < 20 {
			m.error = "Invite code too short"
			return m, nil
		}

		// Ignore if already accepting an invite
		if m.pendingAccept {
			return m, nil
		}

		m.pendingAccept = true
		m.textInput.Blur()
		m.error = ""
		return m, m.acceptInvite(client, code)
	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// viewScan renders the scan invite view.
// Terminal webcam access is not available, so this provides instructions
// for using external QR code scanners or OCR tools.
func (m InvitesModel) viewScan() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(2, 4).
		Width(70)

	var content string
	if m.pendingAccept {
		content = lipgloss.JoinVertical(lipgloss.Center,
			styles.BoxTitle.Render("Accepting Invite..."),
			"",
			m.spinner.View()+" Joining network...",
		)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left,
			styles.BoxTitle.Render("Scan QR Code"),
			"",
			"Terminal webcam scanning is not available.",
			"",
			"To scan a QR code:",
			"1. Use a mobile QR code scanner app, or",
			"2. Use 'zbarimg' command: zbarimg qr.png",
			"3. Paste the extracted i2plan:// URL below",
			"",
			m.textInput.View(),
			"",
		)

		if m.error != "" {
			content = lipgloss.JoinVertical(lipgloss.Left,
				content,
				styles.Error.Render(fmt.Sprintf("⚠ %s", m.error)),
				"",
			)
		}

		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			styles.HelpText.Render("Enter to accept • Esc to cancel"),
		)
	}

	return lipgloss.Place(
		m.width,
		m.height-2,
		lipgloss.Center,
		lipgloss.Center,
		box.Render(content),
	)
}
