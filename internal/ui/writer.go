package ui

import (
	"fmt"
	"io"

	lipgloss "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/colorprofile"
)

// DiffOp represents the type of change in an import plan.
type DiffOp string

const (
	DiffAdd      DiffOp = "Add"
	DiffSkip     DiffOp = "Skip"
	DiffConflict DiffOp = "Conflict"
	DiffRemove   DiffOp = "Remove"
)

// Icons used for message prefixes.
const (
	iconInfo    = "•"
	iconSuccess = "✓"
	iconWarn    = "⚠"
	iconError   = "✗"
)

// Writer provides styled, color-coded CLI output. When JSON mode is enabled,
// all output is silently discarded so that only machine-readable JSON reaches
// stdout.
type Writer struct {
	w          io.Writer
	profile    colorprofile.Profile
	json       bool
	infoStyle  lipgloss.Style
	okStyle    lipgloss.Style
	warnStyle  lipgloss.Style
	errStyle   lipgloss.Style
	faintStyle lipgloss.Style
	boldStyle  lipgloss.Style
	diffStyles map[DiffOp]lipgloss.Style
}

// NewWriter creates a Writer that auto-detects the terminal's color profile.
// If json is true the Writer wraps io.Discard and all methods become no-ops.
func NewWriter(w io.Writer, json bool) *Writer {
	if json {
		return &Writer{w: io.Discard, json: true}
	}
	p := colorprofile.Detect(w, nil)
	return newWriter(w, p)
}

// NewWriterWithProfile creates a Writer with an explicit color profile,
// useful for tests that need deterministic styled output.
func NewWriterWithProfile(w io.Writer, p colorprofile.Profile) *Writer {
	return newWriter(w, p)
}

func newWriter(w io.Writer, p colorprofile.Profile) *Writer {
	pw := &colorprofile.Writer{Forward: w, Profile: p}
	return &Writer{
		w:          pw,
		profile:    p,
		infoStyle:  lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4)), // blue
		okStyle:    lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)), // green
		warnStyle:  lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)), // yellow
		errStyle:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)), // red
		faintStyle: lipgloss.NewStyle().Faint(true),
		boldStyle:  lipgloss.NewStyle().Bold(true),
		diffStyles: map[DiffOp]lipgloss.Style{
			DiffAdd:      lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Bold(true),
			DiffSkip:     lipgloss.NewStyle().Faint(true),
			DiffConflict: lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true),
			DiffRemove:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true),
		},
	}
}

// Info prints a blue informational message prefixed with "•".
func (w *Writer) Info(format string, a ...any) {
	if w.json {
		return
	}
	w.printMsg(w.infoStyle, iconInfo, format, a...)
}

// Success prints a green success message prefixed with "✓".
func (w *Writer) Success(format string, a ...any) {
	if w.json {
		return
	}
	w.printMsg(w.okStyle, iconSuccess, format, a...)
}

// Warn prints a yellow warning message prefixed with "⚠".
func (w *Writer) Warn(format string, a ...any) {
	if w.json {
		return
	}
	w.printMsg(w.warnStyle, iconWarn, format, a...)
}

// Error prints a red error message prefixed with "✗".
func (w *Writer) Error(format string, a ...any) {
	if w.json {
		return
	}
	w.printMsg(w.errStyle, iconError, format, a...)
}

// Abort prints a yellow "Aborted." message.
func (w *Writer) Abort() {
	w.Warn("Aborted.")
}

// DiffLine prints a single line of an import plan with an aligned, colored
// operation label. All labels are padded to the width of the longest op
// ("Conflict") so columns stay aligned.
func (w *Writer) DiffLine(op DiffOp, text string) {
	if w.json {
		return
	}
	style, ok := w.diffStyles[op]
	if !ok {
		_, _ = fmt.Fprintf(w.w, "%-10s%s\n", string(op)+":", text)
		return
	}
	// Pad label to 10 chars (len("Conflict: ")) for alignment.
	label := fmt.Sprintf("%-10s", string(op)+":")
	_, _ = fmt.Fprintf(w.w, "%s%s\n", style.Render(label), text)
}

// DiffList prints an import plan section with one item per line. The first
// item shows the colored operation label; subsequent items are indented to
// align with it.
func (w *Writer) DiffList(op DiffOp, items []string) {
	if w.json || len(items) == 0 {
		return
	}
	for i, item := range items {
		if i == 0 {
			w.DiffLine(op, item)
		} else {
			// 10-char indent to align with the label column.
			_, _ = fmt.Fprintf(w.w, "          %s\n", item)
		}
	}
}

// NothingToDo prints a faint "Nothing to do." message.
func (w *Writer) NothingToDo() {
	if w.json {
		return
	}
	_, _ = fmt.Fprintln(w.w, w.faintStyle.Render("Nothing to do."))
}

// Bold renders text as bold, writing to the given target. The target gets
// the same color profile as the Writer so ANSI codes are downsampled
// correctly even when the target differs from the Writer's own io.Writer
// (e.g. stdout vs stderr).
func (w *Writer) Bold(target io.Writer, format string, a ...any) {
	if w.json {
		return
	}
	pw := &colorprofile.Writer{Forward: target, Profile: w.profile}
	msg := fmt.Sprintf(format, a...)
	_, _ = fmt.Fprint(pw, w.boldStyle.Render(msg))
}

// Table renders a borderless, styled table to the given target writer.
// Headers are bold; columns are padded for alignment. In JSON mode, nothing
// is written.
func (w *Writer) Table(target io.Writer, headers []string, rows [][]string) {
	if w.json {
		return
	}
	cellStyle := lipgloss.NewStyle().PaddingRight(2)
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		BorderRow(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return w.boldStyle.PaddingRight(2)
			}
			return cellStyle
		})
	pw := &colorprofile.Writer{Forward: target, Profile: w.profile}
	_, _ = fmt.Fprintln(pw, t.Render())
}

// List prints each item as a bulleted line, indented under the previous
// message. Useful for making destructive-action warnings scannable.
func (w *Writer) List(items []string) {
	if w.json {
		return
	}
	for _, item := range items {
		_, _ = fmt.Fprintf(w.w, "  %s %s\n", w.faintStyle.Render("•"), item)
	}
}

// Faint prints a faint (dimmed) line with the given indentation prefix.
func (w *Writer) Faint(format string, a ...any) {
	if w.json {
		return
	}
	msg := fmt.Sprintf(format, a...)
	_, _ = fmt.Fprintln(w.w, w.faintStyle.Render(msg))
}

// Writer returns the underlying io.Writer.
func (w *Writer) Writer() io.Writer {
	return w.w
}

func (w *Writer) printMsg(style lipgloss.Style, icon, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	_, _ = fmt.Fprintf(w.w, "%s %s\n", style.Render(icon), msg)
}
