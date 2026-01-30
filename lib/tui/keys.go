package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines the keybindings for the TUI.
type KeyMap struct {
	Quit      key.Binding
	Tab       key.Binding
	ShiftTab  key.Binding
	Refresh   key.Binding
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	Escape    key.Binding
	Peers     key.Binding
	Routes    key.Binding
	Invites   key.Binding
	Status    key.Binding
	Logs      key.Binding
	NewInvite key.Binding
	Accept    key.Binding
	Connect   key.Binding
	Help      key.Binding
}

var keys = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next tab"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev tab"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Peers: key.NewBinding(
		key.WithKeys("1"),
		key.WithHelp("1", "peers"),
	),
	Routes: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "routes"),
	),
	Invites: key.NewBinding(
		key.WithKeys("3"),
		key.WithHelp("3", "invites"),
	),
	Status: key.NewBinding(
		key.WithKeys("4"),
		key.WithHelp("4", "status"),
	),
	Logs: key.NewBinding(
		key.WithKeys("5"),
		key.WithHelp("5", "logs"),
	),
	NewInvite: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new invite"),
	),
	Accept: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "accept invite"),
	),
	Connect: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "connect"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
}
