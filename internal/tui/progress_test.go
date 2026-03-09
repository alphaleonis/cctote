package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestProgressModel_Lifecycle(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewProgress(keys)

	if m.Active() {
		t.Fatal("should start inactive")
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Activate("Applying…", 3, cancel)

	if !m.Active() {
		t.Fatal("should be active after Activate")
	}
	if m.state != progressRunning {
		t.Errorf("state = %d, want %d (running)", m.state, progressRunning)
	}

	// Simulate operations.
	m.HandleUpdate(ProgressUpdateMsg{
		Section: engine.SectionPlugin, Name: "p1", Action: engine.ActionAdded,
		Done: false, Current: 1, Total: 3,
	})
	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.entries))
	}
	if m.entries[0].done {
		t.Error("entry should not be done yet")
	}

	m.HandleUpdate(ProgressUpdateMsg{
		Section: engine.SectionPlugin, Name: "p1", Action: engine.ActionAdded,
		Done: true, Current: 1, Total: 3,
	})
	if !m.entries[0].done {
		t.Error("entry should be done")
	}
	if m.entries[0].err != nil {
		t.Error("entry should have no error")
	}

	// Finish successfully.
	m.HandleFinished(ProgressFinishedMsg{})
	if m.state != progressCompleted {
		t.Errorf("state = %d, want %d (completed)", m.state, progressCompleted)
	}

	m.Deactivate()
	if m.Active() {
		t.Error("should be inactive after Deactivate")
	}
}

func TestProgressModel_ErrorState(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewProgress(keys)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Activate("Test", 1, cancel)

	m.HandleFinished(ProgressFinishedMsg{Err: context.DeadlineExceeded})
	if m.state != progressErrored {
		t.Errorf("state = %d, want %d (errored)", m.state, progressErrored)
	}
}

func TestProgressModel_CancelledState(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewProgress(keys)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Activate("Test", 1, cancel)

	m.HandleFinished(ProgressFinishedMsg{Err: context.Canceled})
	if m.state != progressCancelled {
		t.Errorf("state = %d, want %d (cancelled)", m.state, progressCancelled)
	}
}

func TestProgressModel_ViewOver(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewProgress(keys)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.SetSize(80, 24)
	m.Activate("Applying…", 2, cancel)

	// Should not panic and should produce output containing the title.
	result := m.ViewOver("background content")
	if result == "" {
		t.Error("ViewOver should produce non-empty output")
	}
	if !strings.Contains(result, "Applying") {
		t.Error("ViewOver should contain the title")
	}

	// Inactive should pass through.
	m.Deactivate()
	result = m.ViewOver("pass-through")
	if result != "pass-through" {
		t.Errorf("ViewOver on inactive should pass through, got %q", result)
	}
}

func TestCountBulkOps(t *testing.T) {
	tests := []struct {
		name   string
		target PaneSource
		mcp    *engine.ImportPlan
		plug   *engine.ImportPlan
		full   *FullState
		want   int
	}{
		{
			name:   "manifest target is always 1",
			target: PaneSource{Kind: SourceManifest},
			mcp:    &engine.ImportPlan{Add: []string{"a", "b"}},
			plug:   &engine.ImportPlan{Add: []string{"c"}},
			want:   1,
		},
		{
			name:   "claude code with MCP and plugins",
			target: PaneSource{Kind: SourceClaudeCode},
			mcp:    &engine.ImportPlan{Add: []string{"a"}, Conflict: []string{"b"}},
			plug:   &engine.ImportPlan{Add: []string{"p1"}, Remove: []string{"p2"}, Conflict: []string{"p3"}},
			want:   4, // 1 MCP + 3 plugins
		},
		{
			name:   "no MCP changes",
			target: PaneSource{Kind: SourceClaudeCode},
			mcp:    &engine.ImportPlan{},
			plug:   &engine.ImportPlan{Add: []string{"p1", "p2"}},
			want:   2,
		},
		{
			name:   "no changes at all",
			target: PaneSource{Kind: SourceClaudeCode},
			mcp:    &engine.ImportPlan{},
			plug:   &engine.ImportPlan{},
			want:   0,
		},
		{
			name:   "counts marketplace auto-imports",
			target: PaneSource{Kind: SourceClaudeCode},
			mcp:    &engine.ImportPlan{},
			plug:   &engine.ImportPlan{Add: []string{"p1@mp1", "p2@mp1", "p3@mp2"}},
			full: &FullState{
				Manifest: &manifest.Manifest{
					Marketplaces: map[string]manifest.Marketplace{
						"mp1": {Source: "github", Repo: "o/r"},
						"mp2": {Source: "github", Repo: "o2/r2"},
					},
				},
				MktInstalled: map[string]manifest.Marketplace{},
			},
			want: 5, // 2 marketplace auto-imports + 3 plugin adds
		},
		{
			name:   "skips already-installed marketplaces",
			target: PaneSource{Kind: SourceClaudeCode},
			mcp:    &engine.ImportPlan{},
			plug:   &engine.ImportPlan{Add: []string{"p1@mp1"}},
			full: &FullState{
				Manifest: &manifest.Manifest{
					Marketplaces: map[string]manifest.Marketplace{
						"mp1": {Source: "github", Repo: "o/r"},
					},
				},
				MktInstalled: map[string]manifest.Marketplace{
					"mp1": {Source: "github", Repo: "o/r"},
				},
			},
			want: 1, // only the plugin add
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countBulkOps(tt.target, tt.mcp, tt.plug, tt.full)
			if got != tt.want {
				t.Errorf("countBulkOps() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestActionVerb(t *testing.T) {
	tests := []struct {
		action engine.ActionKind
		want   string
	}{
		{engine.ActionAdded, "install"},
		{engine.ActionRemoved, "remove "},
		{engine.ActionUpdated, "update "},
		{engine.ActionSkipped, "skipped"},
	}
	for _, tt := range tests {
		got := actionVerb(tt.action)
		if got != tt.want {
			t.Errorf("actionVerb(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestSectionIcon(t *testing.T) {
	if sectionIcon(engine.SectionMCP) != IconMCP {
		t.Errorf("expected MCP icon")
	}
	if sectionIcon(engine.SectionPlugin) != IconPlugin {
		t.Errorf("expected Plugin icon")
	}
	if sectionIcon(engine.SectionMarketplace) != IconMarketplace {
		t.Errorf("expected Marketplace icon")
	}
}
