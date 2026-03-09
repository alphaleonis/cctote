package tui

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// HelpBar renders a bottom-anchored help bar from the given groups.
// Each group is a two-sub-column block (key | description) separated by dim │.
// A dim ─ rule spans the full width above the columns.
func HelpBar(groups []HelpGroup, width int) string {
	if len(groups) == 0 || width < 10 {
		return ""
	}

	// Render each group into lines of "key  desc".
	type groupLines struct {
		name  string
		lines []string
	}
	var rendered []groupLines
	maxLines := 0
	for _, g := range groups {
		var lines []string
		for _, b := range g.Bindings {
			h := b.Binding.Help()
			k := helpPadKey(h.Key, 8)
			if b.Dimmed {
				line := StyleItemDim.Render(k + h.Desc)
				lines = append(lines, line)
			} else {
				line := StyleBold.Render(k) + StyleHint.Render(h.Desc)
				lines = append(lines, line)
			}
		}
		rendered = append(rendered, groupLines{name: g.Name, lines: lines})
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}

	// Pad all groups to the same height.
	for i := range rendered {
		for len(rendered[i].lines) < maxLines {
			rendered[i].lines = append(rendered[i].lines, "")
		}
	}

	// Measure each column's natural width (max of header and all binding lines).
	colWidths := make([]int, len(rendered))
	for i, g := range rendered {
		w := lipgloss.Width(StyleSectionHeader.Render(g.name))
		if w > colWidths[i] {
			colWidths[i] = w
		}
		for _, line := range g.lines {
			if lw := lipgloss.Width(line); lw > colWidths[i] {
				colWidths[i] = lw
			}
		}
	}

	// Build rows by joining columns with a dim separator.
	sep := StyleItemDim.Render(" │ ")

	var rows []string
	// Header row with group names.
	var headerParts []string
	for i, g := range rendered {
		headerParts = append(headerParts, helpPadRight(StyleSectionHeader.Render(g.name), colWidths[i]))
	}
	rows = append(rows, strings.Join(headerParts, sep))

	// Binding rows.
	for row := 0; row < maxLines; row++ {
		var parts []string
		for i, g := range rendered {
			parts = append(parts, helpPadRight(g.lines[row], colWidths[i]))
		}
		rows = append(rows, strings.Join(parts, sep))
	}

	rule := StyleItemDim.Render(strings.Repeat("─", width))
	return rule + "\n" + strings.Join(rows, "\n")
}

// HelpBarHeight returns the number of terminal lines the help bar will occupy.
// Returns 0 if no groups. Otherwise: 1 (rule) + 1 (header) + max(bindings per group).
func HelpBarHeight(groups []HelpGroup) int {
	if len(groups) == 0 {
		return 0
	}
	maxBindings := 0
	for _, g := range groups {
		if len(g.Bindings) > maxBindings {
			maxBindings = len(g.Bindings)
		}
	}
	return 1 + 1 + maxBindings // rule + header + binding rows
}

func helpPadKey(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func helpPadRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
