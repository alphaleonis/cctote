package tui

import (
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestMaskValue(t *testing.T) {
	d := NewDetail()

	// Default: secrets hidden.
	if got := d.maskValue("my-secret-key"); got != maskPlaceholder {
		t.Errorf("maskValue with secrets hidden: got %q, want %q", got, maskPlaceholder)
	}

	// After reveal: returns actual value.
	d.ToggleSecrets()
	if got := d.maskValue("my-secret-key"); got != "my-secret-key" {
		t.Errorf("maskValue with secrets revealed: got %q, want %q", got, "my-secret-key")
	}

	// Toggle back: masked again.
	d.ToggleSecrets()
	if got := d.maskValue("my-secret-key"); got != maskPlaceholder {
		t.Errorf("maskValue after re-hide: got %q, want %q", got, maskPlaceholder)
	}
}

func TestHasSecrets(t *testing.T) {
	d := NewDetail()

	// No item set — no secrets.
	if d.HasSecrets() {
		t.Error("HasSecrets should be false with no item")
	}

	// MCP item with env vars.
	d.SetItem(
		&PanelItem{Section: SectionMCP, Name: "test-server"},
		&ItemSync{
			Status: Synced,
			Left:   manifest.MCPServer{Env: map[string]string{"API_KEY": "secret123"}},
			Right:  manifest.MCPServer{Env: map[string]string{"API_KEY": "secret123"}},
		},
	)
	if !d.HasSecrets() {
		t.Error("HasSecrets should be true for MCP item with env vars")
	}

	// MCP item with headers.
	d.SetItem(
		&PanelItem{Section: SectionMCP, Name: "test-server"},
		&ItemSync{
			Status: Synced,
			Left:   manifest.MCPServer{Headers: map[string]string{"Authorization": "Bearer tok"}},
			Right:  manifest.MCPServer{Headers: map[string]string{"Authorization": "Bearer tok"}},
		},
	)
	if !d.HasSecrets() {
		t.Error("HasSecrets should be true for MCP item with headers")
	}

	// MCP item without env or headers.
	d.SetItem(
		&PanelItem{Section: SectionMCP, Name: "test-server"},
		&ItemSync{
			Status: Synced,
			Left:   manifest.MCPServer{Command: "npx"},
			Right:  manifest.MCPServer{Command: "npx"},
		},
	)
	if d.HasSecrets() {
		t.Error("HasSecrets should be false for MCP item without env/headers")
	}

	// Plugin item — never has secrets.
	d.SetItem(
		&PanelItem{Section: SectionPlugin, Name: "test-plugin"},
		&ItemSync{
			Status: Synced,
			Left:   manifest.Plugin{ID: "test", Scope: "global", Enabled: true},
			Right:  manifest.Plugin{ID: "test", Scope: "global", Enabled: true},
		},
	)
	if d.HasSecrets() {
		t.Error("HasSecrets should be false for plugin items")
	}
}

func TestSetItemResetsSecrets(t *testing.T) {
	d := NewDetail()
	d.SetSize(80, 10)

	// Set an MCP item with secrets and reveal them.
	d.SetItem(
		&PanelItem{Section: SectionMCP, Name: "server-a", SyncStatus: Synced},
		&ItemSync{
			Status: Synced,
			Left:   manifest.MCPServer{Env: map[string]string{"KEY": "secret-a"}},
			Right:  manifest.MCPServer{Env: map[string]string{"KEY": "secret-a"}},
		},
	)
	d.ToggleSecrets()
	if !d.secretsRevealed {
		t.Fatal("secretsRevealed should be true after toggle")
	}

	// Navigate to a different item — secrets should be re-masked.
	d.SetItem(
		&PanelItem{Section: SectionMCP, Name: "server-b", SyncStatus: Synced},
		&ItemSync{
			Status: Synced,
			Left:   manifest.MCPServer{Env: map[string]string{"KEY": "secret-b"}},
			Right:  manifest.MCPServer{Env: map[string]string{"KEY": "secret-b"}},
		},
	)
	if d.secretsRevealed {
		t.Error("secretsRevealed should be reset to false after SetItem")
	}
}

func TestRenderEnvMapMasked(t *testing.T) {
	d := NewDetail()

	lines := d.renderEnvMap("Env", map[string]string{
		"API_KEY":     "sk-12345",
		"DB_PASSWORD": "hunter2",
	})

	// Values should be masked.
	for _, line := range lines {
		if strings.Contains(line, "sk-12345") || strings.Contains(line, "hunter2") {
			t.Errorf("masked output should not contain secret values: %q", line)
		}
		if !strings.Contains(line, maskPlaceholder) {
			t.Errorf("masked output should contain placeholder: %q", line)
		}
	}

	// After revealing secrets, values should appear.
	d.ToggleSecrets()
	lines = d.renderEnvMap("Env", map[string]string{
		"API_KEY": "sk-12345",
	})
	found := false
	for _, line := range lines {
		if strings.Contains(line, "sk-12345") {
			found = true
		}
	}
	if !found {
		t.Error("revealed output should contain actual secret values")
	}
}

func TestDiffEnvMapMasked(t *testing.T) {
	d := NewDetail()

	left := map[string]string{"TOKEN": "old-secret"}
	right := map[string]string{"TOKEN": "new-secret"}

	lines := d.diffEnvMap("Headers", left, right)

	for _, line := range lines {
		if strings.Contains(line, "old-secret") || strings.Contains(line, "new-secret") {
			t.Errorf("masked diff should not contain secret values: %q", line)
		}
	}
}
