package tui

import "charm.land/bubbles/v2/key"

// KeyMap defines all key bindings for the TUI.
type KeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	Tab       key.Binding
	Top       key.Binding
	Bottom    key.Binding
	SectMCP   key.Binding
	SectPlug  key.Binding
	SectMkt   key.Binding
	Refresh   key.Binding
	Help      key.Binding
	Quit      key.Binding
	ForceQuit key.Binding

	// Actions
	// Bulk import/export (I/E) removed in favor of V (select section) +
	// CopyRight/CopyLeft. This unifies single and bulk operations under
	// one directional model.
	CopyRight key.Binding // ctrl+right, > — copy to right pane
	CopyLeft  key.Binding // ctrl+left, < — copy to left pane
	Select    key.Binding // v/space — toggle multi-select
	SelectSect key.Binding // V — select all in section
	Delete     key.Binding // x/Delete — delete from focused pane's source
	Enable     key.Binding // e — enable plugin
	Disable    key.Binding // d — disable plugin
	UpdateMkt  key.Binding // u — update/refresh marketplace

	// Sources
	SourceLeft    key.Binding // f1 — pick left pane source
	SourceRight   key.Binding // f2 — pick right pane source
	BulkApply     key.Binding // A — apply
	CreateProfile key.Binding // N — create profile
	DeleteProfile key.Binding // D — delete profile

	// Config
	Config key.Binding // C — open config overlay

	// Detail
	ToggleSecrets key.Binding // s — toggle secret visibility
	ExpandDetail  key.Binding // enter — expand detail pane
	Filter        key.Binding // / — filter items
	CopyJSON      key.Binding // y — copy MCP server as JSON to clipboard

	// Overlay
	Confirm key.Binding // enter/y — confirm action
	Cancel  key.Binding // esc/n — cancel overlay
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("←/h", "left panel"),
		),
		Right: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("→/l", "right panel"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch panel"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "go to top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to bottom"),
		),
		SectMCP: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "MCP servers"),
		),
		SectPlug: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "plugins"),
		),
		SectMkt: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "marketplaces"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q/esc", "quit"),
		),
		ForceQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "force quit"),
		),

		// Actions
		CopyRight: key.NewBinding(
			key.WithKeys("ctrl+right", ">"),
			key.WithHelp(">/C-→", "copy right"),
		),
		CopyLeft: key.NewBinding(
			key.WithKeys("ctrl+left", "<"),
			key.WithHelp("</C-←", "copy left"),
		),
		Select: key.NewBinding(
			key.WithKeys("v", "space"),
			key.WithHelp("v/space", "select"),
		),
		SelectSect: key.NewBinding(
			key.WithKeys("V"),
			key.WithHelp("V", "select section"),
		),
		Delete: key.NewBinding(
			key.WithKeys("x", "delete"),
			key.WithHelp("x/Del", "delete"),
		),
		Enable: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "enable"),
		),
		Disable: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "disable"),
		),
		UpdateMkt: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "update marketplace"),
		),

		// Sources
		SourceLeft: key.NewBinding(
			key.WithKeys("f1"),
			key.WithHelp("F1", "left source"),
		),
		SourceRight: key.NewBinding(
			key.WithKeys("f2"),
			key.WithHelp("F2", "right source"),
		),
		BulkApply: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "apply"),
		),
		CreateProfile: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "create profile"),
		),
		DeleteProfile: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "delete profile"),
		),

		// Config
		Config: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "config"),
		),

		// Detail
		ToggleSecrets: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "toggle secrets"),
		),
		ExpandDetail: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "expand detail"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		CopyJSON: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy JSON"),
		),

		// Overlay
		Confirm: key.NewBinding(
			key.WithKeys("enter", "y"),
			key.WithHelp("enter/y", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "n"),
			key.WithHelp("esc/n", "cancel"),
		),
	}
}

// ShortHelp returns the key bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Tab, k.Help, k.Quit}
}

// FullHelp returns the key bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Tab},
		{k.Top, k.Bottom, k.SectMCP, k.SectPlug, k.SectMkt},
		{k.Refresh, k.Config, k.ToggleSecrets, k.ExpandDetail, k.Filter, k.CopyJSON, k.Help, k.Quit, k.ForceQuit},
		{k.CopyRight, k.CopyLeft, k.Select, k.SelectSect, k.Delete, k.Enable, k.Disable, k.UpdateMkt},
		{k.SourceLeft, k.SourceRight, k.BulkApply, k.CreateProfile, k.DeleteProfile},
	}
}

// HelpContext captures the current TUI state for context-sensitive help.
type HelpContext struct {
	FilterActive   bool
	DetailExpanded bool
	OverlayActive  bool
	Ready          bool
	Busy           bool
	HasProfile     bool
}

// HelpGroup is a named column of bindings for the help bar.
type HelpGroup struct {
	Name     string
	Bindings []HelpBinding
}

// HelpBinding pairs a key binding with a dimmed flag for conditional availability.
type HelpBinding struct {
	Binding key.Binding
	Dimmed  bool
}

// ContextualHelp returns help groups appropriate for the current context.
func (k KeyMap) ContextualHelp(ctx HelpContext) []HelpGroup {
	if ctx.OverlayActive {
		return []HelpGroup{{
			Name: "Overlay",
			Bindings: []HelpBinding{
				{Binding: k.Confirm},
				{Binding: k.Cancel},
			},
		}}
	}

	if ctx.FilterActive {
		return []HelpGroup{{
			Name: "Filter",
			Bindings: []HelpBinding{
				{Binding: key.NewBinding(key.WithHelp("enter", "apply"))},
				{Binding: key.NewBinding(key.WithHelp("esc", "clear"))},
			},
		}}
	}

	if ctx.DetailExpanded {
		return []HelpGroup{{
			Name: "Detail",
			Bindings: []HelpBinding{
				{Binding: k.Up},
				{Binding: k.Down},
				{Binding: k.ToggleSecrets},
				{Binding: k.CopyJSON},
				{Binding: k.Enable},
				{Binding: k.Disable},
				{Binding: key.NewBinding(key.WithHelp("q", "collapse"))},
				{Binding: k.Help},
			},
		}}
	}

	actionsDimmed := !ctx.Ready || ctx.Busy
	sourcesDimmed := !ctx.Ready || ctx.Busy

	return []HelpGroup{
		{
			Name: "Navigation",
			Bindings: []HelpBinding{
				{Binding: k.Up},
				{Binding: k.Down},
				{Binding: k.Left},
				{Binding: k.Right},
				{Binding: k.Tab},
			},
		},
		{
			Name: "Actions",
			Bindings: []HelpBinding{
				{Binding: k.CopyRight, Dimmed: actionsDimmed},
				{Binding: k.CopyLeft, Dimmed: actionsDimmed},
				{Binding: k.Select, Dimmed: actionsDimmed},
				{Binding: k.SelectSect, Dimmed: actionsDimmed},
				{Binding: k.Delete, Dimmed: actionsDimmed},
				{Binding: k.Enable, Dimmed: actionsDimmed},
				{Binding: k.Disable, Dimmed: actionsDimmed},
				{Binding: k.UpdateMkt, Dimmed: actionsDimmed},
			},
		},
		{
			Name: "Sources",
			Bindings: []HelpBinding{
				{Binding: k.SourceLeft, Dimmed: sourcesDimmed},
				{Binding: k.SourceRight, Dimmed: sourcesDimmed},
				{Binding: k.BulkApply, Dimmed: sourcesDimmed},
				{Binding: k.CreateProfile, Dimmed: sourcesDimmed},
				{Binding: k.DeleteProfile, Dimmed: sourcesDimmed || !ctx.HasProfile},
			},
		},
		{
			Name: "General",
			Bindings: []HelpBinding{
				{Binding: k.Refresh},
				{Binding: k.Config},
				{Binding: k.Filter},
				{Binding: k.CopyJSON},
				{Binding: k.Help},
				{Binding: k.Quit},
			},
		},
	}
}
