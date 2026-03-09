// Package tui implements the interactive terminal UI for cctote.
package tui

import lipgloss "charm.land/lipgloss/v2"

// Color palette — punched-up from VS Code Dark+ for better TUI readability.
var (
	ColorBG        = lipgloss.Color("#1e1e1e")
	ColorFG        = lipgloss.Color("#e0e0e0")
	ColorDim       = lipgloss.Color("#808080")
	ColorFGDim     = lipgloss.Color("#a0a0a0") // slightly dimmer than FG, for synced items
	ColorHighlight = lipgloss.Color("#d8cc8c") // pale yellow for linked/highlighted items
	ColorBorder    = lipgloss.Color("#505050")
	ColorFocusBdr  = lipgloss.Color("#1a9fff")
	ColorBlue      = lipgloss.Color("#6cb6ff")
	ColorGreen     = lipgloss.Color("#58d68d")
	ColorYellow    = lipgloss.Color("#f0d060")
	ColorOrange    = lipgloss.Color("#f0a050")
	ColorRed       = lipgloss.Color("#ff6b6b")
	ColorCyan      = lipgloss.Color("#56d4c0")
	ColorMagenta   = lipgloss.Color("#da7bda")
	ColorSelection = lipgloss.Color("#264f78")
	ColorMultiSel  = lipgloss.Color("#3a3d41")

	// Source-type border colors (focused/bright variants).
	ColorSrcManifest   = lipgloss.Color("#1a9fff") // blue
	ColorSrcProfile    = lipgloss.Color("#58d68d") // green
	ColorSrcClaudeCode = lipgloss.Color("#da7b27") // orange (Claude logo)
	ColorSrcProject    = lipgloss.Color("#da7bda") // magenta — project config

	// Source-type border colors (unfocused/dim variants).
	ColorSrcManifestDim   = lipgloss.Color("#0d5090")
	ColorSrcProfileDim    = lipgloss.Color("#2c6b47")
	ColorSrcClaudeCodeDim = lipgloss.Color("#6d3d13")
	ColorSrcProjectDim    = lipgloss.Color("#6d3d6d")
)

// Pre-built border styles.
var (
	FocusedBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorFocusBdr)

	UnfocusedBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)
)

// Text styles.
var (
	StyleSectionHeader = lipgloss.NewStyle().Bold(true).Foreground(ColorBlue)
	StyleItemNormal    = lipgloss.NewStyle().Foreground(ColorFG)
	StyleItemDim       = lipgloss.NewStyle().Foreground(ColorDim)
	StyleItemSynced    = lipgloss.NewStyle().Foreground(ColorFGDim)
	StyleItemHighlight = lipgloss.NewStyle().Foreground(ColorHighlight)
	StyleItemSelected  = lipgloss.NewStyle().Background(ColorSelection).Foreground(ColorFG)
	StyleItemMultiSel  = lipgloss.NewStyle().Background(ColorMultiSel).Foreground(ColorFG)
	StyleSelectIcon    = lipgloss.NewStyle().Foreground(ColorBlue)
	StyleLinkedIcon    = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleStatusSynced  = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleStatusDiff    = lipgloss.NewStyle().Foreground(ColorYellow)
	StyleStatusLeft    = lipgloss.NewStyle().Foreground(ColorCyan)
	StyleStatusRight   = lipgloss.NewStyle().Foreground(ColorOrange)
	StyleDiffAdd       = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleDiffRemove    = lipgloss.NewStyle().Foreground(ColorRed)
	StyleHint          = lipgloss.NewStyle().Foreground(ColorDim)
	StyleBold          = lipgloss.NewStyle().Bold(true)

	// Confirmation overlay styles.
	StyleConfirmBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorFocusBdr)
	StyleButtonFocused = lipgloss.NewStyle().
				Background(ColorFocusBdr).
				Foreground(ColorFG).
				Padding(0, 1)
	StyleButtonNormal = lipgloss.NewStyle().
				Foreground(ColorFG).
				Padding(0, 1)
	StyleButtonDanger = lipgloss.NewStyle().
				Background(ColorRed).
				Foreground(ColorFG).
				Padding(0, 1)

	// Flash message styles.
	StyleFlashSuccess = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleFlashError   = lipgloss.NewStyle().Foreground(ColorRed)
	StyleFlashInfo    = lipgloss.NewStyle().Foreground(ColorCyan)
)

// Nerd Font icons.
const (
	IconMCP         = "󰒍"
	IconPlugin      = "󰐱"
	IconMarketplace = "󰏬"
	IconConfig      = "󰒓"
	IconManifest    = "󰈙" // document — for manifest source
	IconClaudeCode  = "󰘬" // terminal — for Claude Code source
	IconProfile     = "󰓾" // person — for profile source
	IconProject     = "󰊢" // git branch — for project source
	IconSynced      = "✓"
	IconDifferent   = "≠"
	IconSelected    = "●"
	IconLinked      = "◆"
)
