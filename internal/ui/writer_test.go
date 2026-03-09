package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

// plainWriter creates a Writer that outputs no ANSI escapes (Ascii profile).
func plainWriter(buf *bytes.Buffer) *Writer {
	return NewWriterWithProfile(buf, colorprofile.Ascii)
}

// colorWriter creates a Writer with TrueColor so ANSI sequences are emitted.
func colorWriter(buf *bytes.Buffer) *Writer {
	return NewWriterWithProfile(buf, colorprofile.TrueColor)
}

func TestInfoPlain(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.Info("hello %s", "world")
	got := buf.String()
	if !strings.Contains(got, "•") {
		t.Errorf("expected info icon, got %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("expected message, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("expected trailing newline, got %q", got)
	}
}

func TestSuccessPlain(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.Success("exported %d", 5)
	got := buf.String()
	if !strings.Contains(got, "✓") {
		t.Errorf("expected success icon, got %q", got)
	}
	if !strings.Contains(got, "exported 5") {
		t.Errorf("expected message, got %q", got)
	}
}

func TestWarnPlain(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.Warn("careful %q", "fire")
	got := buf.String()
	if !strings.Contains(got, "⚠") {
		t.Errorf("expected warn icon, got %q", got)
	}
	if !strings.Contains(got, `careful "fire"`) {
		t.Errorf("expected message, got %q", got)
	}
}

func TestErrorPlain(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.Error("bad things")
	got := buf.String()
	if !strings.Contains(got, "✗") {
		t.Errorf("expected error icon, got %q", got)
	}
	if !strings.Contains(got, "bad things") {
		t.Errorf("expected message, got %q", got)
	}
}

func TestAbort(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.Abort()
	got := buf.String()
	if !strings.Contains(got, "⚠") {
		t.Errorf("expected warn icon, got %q", got)
	}
	if !strings.Contains(got, "Aborted.") {
		t.Errorf("expected Aborted message, got %q", got)
	}
}

func TestJSONModeSuppressesAll(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, true)
	w.Info("should not appear")
	w.Success("should not appear")
	w.Warn("should not appear")
	w.Error("should not appear")
	w.Abort()
	w.DiffLine(DiffAdd, "server-a")
	w.NothingToDo()
	if buf.Len() != 0 {
		t.Errorf("expected no output in JSON mode, got %q", buf.String())
	}
}

func TestDiffLinePlain(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.DiffLine(DiffAdd, "server-a, server-b")
	w.DiffLine(DiffSkip, "server-c")
	w.DiffLine(DiffConflict, "server-d")
	w.DiffLine(DiffRemove, "server-e")
	got := buf.String()

	for _, want := range []string{
		"Add:",
		"Skip:",
		"Conflict:",
		"Remove:",
		"server-a, server-b",
		"server-c",
		"server-d",
		"server-e",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}

	// Verify alignment: all text portions start at the same column.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}
}

func TestNothingToDoPlain(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.NothingToDo()
	got := buf.String()
	if !strings.Contains(got, "Nothing to do.") {
		t.Errorf("expected nothing-to-do message, got %q", got)
	}
}

func TestColorOutputContainsANSI(t *testing.T) {
	var buf bytes.Buffer
	w := colorWriter(&buf)
	w.Info("colored")
	got := buf.String()
	// ANSI escape sequences start with ESC[
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("expected ANSI escape codes in colored output, got %q", got)
	}
	if !strings.Contains(got, "colored") {
		t.Errorf("expected message text in output, got %q", got)
	}
}

func TestDiffLineColorContainsANSI(t *testing.T) {
	var buf bytes.Buffer
	w := colorWriter(&buf)
	w.DiffLine(DiffAdd, "new-server")
	got := buf.String()
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("expected ANSI escape codes, got %q", got)
	}
	if !strings.Contains(got, "new-server") {
		t.Errorf("expected server name, got %q", got)
	}
}

func TestDiffLineUnknownOp(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.DiffLine(DiffOp("Custom"), "item-x")
	got := buf.String()
	if !strings.Contains(got, "Custom:") {
		t.Errorf("expected fallback label, got %q", got)
	}
	if !strings.Contains(got, "item-x") {
		t.Errorf("expected item text, got %q", got)
	}
}

func TestDiffListMultipleItems(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.DiffList(DiffAdd, []string{"first", "second", "third"})
	got := buf.String()

	if !strings.Contains(got, "Add:") {
		t.Errorf("expected Add label on first line, got %q", got)
	}
	if !strings.Contains(got, "first") {
		t.Errorf("expected first item, got %q", got)
	}
	if !strings.Contains(got, "second") {
		t.Errorf("expected second item, got %q", got)
	}
	if !strings.Contains(got, "third") {
		t.Errorf("expected third item, got %q", got)
	}

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}

	// Second and third lines should be indented (10 spaces), not labeled.
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "          ") {
			t.Errorf("expected 10-space indent, got %q", line)
		}
	}
}

func TestDiffListEmpty(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.DiffList(DiffAdd, nil)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty list, got %q", buf.String())
	}
}

func TestDiffListSingleItem(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.DiffList(DiffRemove, []string{"only-one"})
	got := buf.String()
	if !strings.Contains(got, "Remove:") {
		t.Errorf("expected Remove label, got %q", got)
	}
	if !strings.Contains(got, "only-one") {
		t.Errorf("expected item, got %q", got)
	}
}

func TestBoldPlain(t *testing.T) {
	var buf bytes.Buffer
	var target bytes.Buffer
	w := plainWriter(&buf)
	w.Bold(&target, "hello %s", "bold")
	got := target.String()
	if !strings.Contains(got, "hello bold") {
		t.Errorf("expected bold text, got %q", got)
	}
}

func TestBoldJSONMode(t *testing.T) {
	var buf bytes.Buffer
	var target bytes.Buffer
	w := NewWriter(&buf, true)
	w.Bold(&target, "should not appear")
	if target.Len() != 0 {
		t.Errorf("expected no output in JSON mode, got %q", target.String())
	}
}

func TestTablePlain(t *testing.T) {
	var target bytes.Buffer
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.Table(&target, []string{"Name", "Value"}, [][]string{
		{"key1", "val1"},
		{"key2", "val2"},
	})
	got := target.String()
	for _, want := range []string{"Name", "Value", "key1", "val1", "key2", "val2"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in table output, got:\n%s", want, got)
		}
	}
}

func TestTableJSONMode(t *testing.T) {
	var target bytes.Buffer
	var buf bytes.Buffer
	w := NewWriter(&buf, true)
	w.Table(&target, []string{"H"}, [][]string{{"v"}})
	if target.Len() != 0 {
		t.Errorf("expected no output in JSON mode, got %q", target.String())
	}
}

func TestListPlain(t *testing.T) {
	var buf bytes.Buffer
	w := plainWriter(&buf)
	w.List([]string{"alpha", "beta", "gamma"})
	got := buf.String()
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in list output, got:\n%s", want, got)
		}
	}
	// Each item should be on its own line with a bullet.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}

func TestListJSONMode(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, true)
	w.List([]string{"should", "not", "appear"})
	if buf.Len() != 0 {
		t.Errorf("expected no output in JSON mode, got %q", buf.String())
	}
}
