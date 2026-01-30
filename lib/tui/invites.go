package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-i2p/wireguard/lib/rpc"
)

// InvitesMode represents the current mode of the invites view.
type InvitesMode int

const (
	InvitesModeNormal InvitesMode = iota
	InvitesModeCreate
	InvitesModeAccept
)

// InvitesModel is the model for the invites view.
type InvitesModel struct {
	mode          InvitesMode
	width         int
	height        int
	textInput     textinput.Model
	createdInvite *rpc.InviteCreateResult
	acceptResult  *rpc.InviteAcceptResult
	error         string
	message       string
	pendingCreate bool // Tracks if invite creation is in progress
	pendingAccept bool // Tracks if invite acceptance is in progress
}

// NewInvitesModel creates a new invites view model.
func NewInvitesModel() InvitesModel {
	ti := textinput.New()
	ti.Placeholder = "Paste invite code here..."
	ti.CharLimit = 2000
	ti.Width = 60

	return InvitesModel{
		textInput: ti,
	}
}

// SetDimensions sets the view dimensions.
func (m *InvitesModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// SetCreatedInvite sets the created invite result.
func (m *InvitesModel) SetCreatedInvite(invite *rpc.InviteCreateResult) {
	// Only update if we were actually waiting for this result
	if !m.pendingCreate {
		return
	}
	m.pendingCreate = false
	m.createdInvite = invite
	m.error = ""
}

// SetAcceptResult sets the accept result.
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

// Update handles messages for the invites view.
func (m InvitesModel) Update(msg tea.KeyMsg, client *rpc.Client) (InvitesModel, tea.Cmd) {
	switch m.mode {
	case InvitesModeNormal:
		return m.handleNormalModeKey(msg, client)
	case InvitesModeCreate:
		return m.handleCreateModeKey(msg)
	case InvitesModeAccept:
		return m.handleAcceptModeKey(msg, client)
	}
	return m, nil
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
		if code != "" {
			// Ignore if already accepting an invite
			if m.pendingAccept {
				return m, nil
			}
			m.pendingAccept = true
			m.textInput.Blur()
			return m, m.acceptInvite(client, code)
		}
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
		b.WriteString(styles.Error.Render("Error: " + m.error))
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
		styles.Error.Render("Error: "+m.error),
		"",
		styles.HelpText.Render("Press Esc to close"),
	)
}

// buildInviteLoadingContent creates the content while invite is being created.
func (m InvitesModel) buildInviteLoadingContent() string {
	return lipgloss.JoinVertical(lipgloss.Center,
		styles.BoxTitle.Render("Creating Invite..."),
		"",
		"Generating invite code...",
	)
}

// viewAccept renders the accept invite view.
func (m InvitesModel) viewAccept() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(2, 4).
		Width(70)

	content := lipgloss.JoinVertical(lipgloss.Left,
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
			styles.Error.Render("Error: "+m.error),
			"",
		)
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		styles.HelpText.Render("Enter to accept • Esc to cancel"),
	)

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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.InviteAccept(ctx, code)
		if err != nil {
			return errMsg{err: err}
		}
		return inviteAcceptedMsg{result: result}
	}
}
