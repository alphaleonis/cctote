package tui

// LayoutMode indicates how panels are arranged based on terminal width.
type LayoutMode int

const (
	LayoutDual      LayoutMode = iota // >= 100 columns: side-by-side panels
	LayoutSingle                      // 60-99 columns: single panel with tab switching
	LayoutTooNarrow                   // < 60 columns: warning message
)

// Layout holds the computed dimensions for the TUI components.
type Layout struct {
	Mode         LayoutMode
	PanelWidth   int
	PanelHeight  int
	DetailHeight int
	HelpHeight   int
	TotalWidth   int
	TotalHeight  int
}

// ComputeLayout calculates the layout dimensions from terminal size.
// helpHeight is the number of lines the help bar occupies (0 when hidden).
func ComputeLayout(width, height, helpHeight int) Layout {
	l := Layout{
		TotalWidth:  width,
		TotalHeight: height,
		HelpHeight:  helpHeight,
	}

	// Width thresholds
	switch {
	case width >= 100:
		l.Mode = LayoutDual
		// Each panel gets half the width minus 1 for the gap.
		l.PanelWidth = (width - 1) / 2
	case width >= 60:
		l.Mode = LayoutSingle
		l.PanelWidth = width
	default:
		l.Mode = LayoutTooNarrow
		return l
	}

	// Detail height based on terminal height.
	// Reserve 1 line for status bar.
	switch {
	case height >= 24:
		l.DetailHeight = 5
	case height >= 16:
		l.DetailHeight = 2
	default:
		l.DetailHeight = 0
	}

	// Panel height = total - detail - status line (1) - help bar - detail border (2 if detail > 0)
	used := 1 + helpHeight // status line + help bar
	if l.DetailHeight > 0 {
		used += l.DetailHeight + 2 // detail content + top/bottom border
	}
	l.PanelHeight = height - used
	if l.PanelHeight < 3 {
		l.PanelHeight = 3
	}

	return l
}
