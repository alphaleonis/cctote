package cmd

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/config"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/charmbracelet/colorprofile"
)

// newChezmoiApp creates a minimal App with chezmoi seams configured for unit tests.
// chezmoiReAdd is intentionally left nil — tests that exercise the auto-re-add path
// must set it explicitly.
func newChezmoiApp(cfg *config.Config, managed bool) *App {
	return &App{
		appConfig:      cfg,
		chezmoiManaged: func(context.Context, string) bool { return managed },
	}
}

// --- Unit tests for notifyChezmoi ---

func TestNotifyChezmoi_Disabled(t *testing.T) {
	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)

	// nil config — should not even check managed status.
	app := newChezmoiApp(nil, true)
	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/path/manifest.json")
	if buf.Len() > 0 {
		t.Errorf("expected no output for nil config, got: %s", buf.String())
	}

	// enabled = false (explicit)
	buf.Reset()
	app = newChezmoiApp(&config.Config{Chezmoi: config.ChezmoiConfig{Enabled: config.BoolPtr(false)}}, true)
	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/path/manifest.json")
	if buf.Len() > 0 {
		t.Errorf("expected no output for disabled config, got: %s", buf.String())
	}

	// enabled = nil (unset)
	buf.Reset()
	app = newChezmoiApp(&config.Config{}, true)
	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/path/manifest.json")
	if buf.Len() > 0 {
		t.Errorf("expected no output for unset config, got: %s", buf.String())
	}
}

func TestNotifyChezmoi_NotManaged(t *testing.T) {
	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)
	cfg := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled: config.BoolPtr(true),
	}}

	app := newChezmoiApp(cfg, false)
	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/path/manifest.json")
	if buf.Len() > 0 {
		t.Errorf("expected no output for unmanaged file, got: %s", buf.String())
	}
}

func TestNotifyChezmoi_Reminder(t *testing.T) {
	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)
	cfg := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled: config.BoolPtr(true),
	}}

	app := newChezmoiApp(cfg, true)
	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/home/user/.config/cctote/manifest.json")

	out := buf.String()
	if !strings.Contains(out, "chezmoi re-add") {
		t.Errorf("expected reminder message, got: %s", out)
	}
	if !strings.Contains(out, "/home/user/.config/cctote/manifest.json") {
		t.Errorf("expected manifest path in reminder, got: %s", out)
	}
}

func TestNotifyChezmoi_Always_Success(t *testing.T) {
	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)
	cfg := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled:   config.BoolPtr(true),
		AutoReAdd: config.StrPtr("always"),
	}}

	var capturedPath string
	app := newChezmoiApp(cfg, true)
	app.chezmoiReAdd = func(_ context.Context, path string) error {
		capturedPath = path
		return nil
	}

	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/tmp/manifest.json")

	if capturedPath != "/tmp/manifest.json" {
		t.Errorf("expected path %q, got %q", "/tmp/manifest.json", capturedPath)
	}
	out := buf.String()
	if !strings.Contains(out, "Running chezmoi re-add") {
		t.Errorf("expected progress message, got: %s", out)
	}
	if !strings.Contains(out, "Ran chezmoi re-add") {
		t.Errorf("expected success message, got: %s", out)
	}
	// Should NOT contain "Run 'chezmoi" (that's the reminder format).
	if strings.Contains(out, "Run 'chezmoi") {
		t.Errorf("should print success, not reminder: %s", out)
	}
}

func TestNotifyChezmoi_Always_Failure(t *testing.T) {
	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)
	cfg := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled:   config.BoolPtr(true),
		AutoReAdd: config.StrPtr("always"),
	}}

	app := newChezmoiApp(cfg, true)
	app.chezmoiReAdd = func(context.Context, string) error {
		return errors.New("chezmoi: not found")
	}

	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/tmp/manifest.json")

	out := buf.String()
	if !strings.Contains(out, "chezmoi re-add failed") {
		t.Errorf("expected failure warning, got: %s", out)
	}
}

func TestNotifyChezmoi_Ask_Confirmed(t *testing.T) {
	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)
	cfg := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled:   config.BoolPtr(true),
		AutoReAdd: config.StrPtr("ask"),
	}}

	var ran bool
	app := newChezmoiApp(cfg, true)
	app.chezmoiReAdd = func(context.Context, string) error {
		ran = true
		return nil
	}

	app.notifyChezmoi(context.Background(), w, strings.NewReader("y\n"), "/tmp/manifest.json")

	if !ran {
		t.Error("expected chezmoi re-add to run after 'y' confirmation")
	}
	out := buf.String()
	if !strings.Contains(out, "Ran chezmoi re-add") {
		t.Errorf("expected success message, got: %s", out)
	}
}

func TestNotifyChezmoi_Ask_Declined(t *testing.T) {
	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)
	cfg := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled:   config.BoolPtr(true),
		AutoReAdd: config.StrPtr("ask"),
	}}

	var ran bool
	app := newChezmoiApp(cfg, true)
	app.chezmoiReAdd = func(context.Context, string) error {
		ran = true
		return nil
	}

	app.notifyChezmoi(context.Background(), w, strings.NewReader("n\n"), "/tmp/manifest.json")

	if ran {
		t.Error("expected chezmoi re-add NOT to run after 'n'")
	}
}

func TestNotifyChezmoi_StaleConfig(t *testing.T) {
	// Simulate post-TUI scenario: a.appConfig is stale (enabled + "always"),
	// but disk config changed to disabled during TUI session.
	// If a.appConfig is not refreshed, notifyChezmoi incorrectly runs re-add.
	staleConfig := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled:   config.BoolPtr(true),
		AutoReAdd: config.StrPtr("always"),
	}}
	freshConfig := &config.Config{Chezmoi: config.ChezmoiConfig{
		Enabled: config.BoolPtr(false),
	}}

	var ran bool
	app := newChezmoiApp(staleConfig, true)
	app.chezmoiReAdd = func(context.Context, string) error {
		ran = true
		return nil
	}

	// Simulate what the TUI command should do: update appConfig before
	// calling notifyChezmoi.
	app.appConfig = freshConfig

	var buf bytes.Buffer
	w := ui.NewWriterWithProfile(&buf, colorprofile.Ascii)
	app.notifyChezmoi(context.Background(), w, strings.NewReader(""), "/tmp/manifest.json")

	if ran {
		t.Error("notifyChezmoi ran chezmoi re-add with stale config; appConfig should have been refreshed")
	}
	if buf.Len() > 0 {
		t.Errorf("expected no output with disabled config, got: %s", buf.String())
	}
}

// --- Integration tests using execCmd ---

func TestMcpExport_ChezmoiReminder(t *testing.T) {
	home := t.TempDir()
	manPath := filepath.Join(home, ".config", "cctote", "manifest.json")
	cfgFile := filepath.Join(home, ".config", "cctote", "cctote.toml")

	seedClaudeConfig(t, home, map[string]manifest.MCPServer{
		"context7": srvContext7,
	})
	seedConfig(t, cfgFile, &config.Config{
		Chezmoi: config.ChezmoiConfig{Enabled: config.BoolPtr(true)},
	})

	res := execCmdWith(t, home, nil, withChezmoiManaged(true), "--config", cfgFile, "--manifest", manPath, "mcp", "export")
	if res.err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
	}
	if !strings.Contains(res.stderr, "chezmoi re-add") {
		t.Errorf("expected chezmoi reminder in stderr, got: %s", res.stderr)
	}
}

func TestMcpExport_ChezmoiNotManaged(t *testing.T) {
	home := t.TempDir()
	manPath := filepath.Join(home, ".config", "cctote", "manifest.json")
	cfgFile := filepath.Join(home, ".config", "cctote", "cctote.toml")

	seedClaudeConfig(t, home, map[string]manifest.MCPServer{
		"context7": srvContext7,
	})
	seedConfig(t, cfgFile, &config.Config{
		Chezmoi: config.ChezmoiConfig{Enabled: config.BoolPtr(true)},
	})

	res := execCmdWith(t, home, nil, withChezmoiManaged(false), "--config", cfgFile, "--manifest", manPath, "mcp", "export")
	if res.err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
	}
	if strings.Contains(res.stderr, "chezmoi") {
		t.Errorf("expected no chezmoi output for unmanaged file, got: %s", res.stderr)
	}
}

func TestMcpRemove_Aborted_NoChezmoi(t *testing.T) {
	home := t.TempDir()
	manPath := filepath.Join(home, ".config", "cctote", "manifest.json")
	cfgFile := filepath.Join(home, ".config", "cctote", "cctote.toml")

	seedManifest(t, manPath, &manifest.Manifest{
		Version: manifest.CurrentVersion,
		MCPServers: map[string]manifest.MCPServer{
			"context7": srvContext7,
		},
		Profiles: map[string]manifest.Profile{
			"work": {MCPServers: []string{"context7"}},
		},
	})
	seedConfig(t, cfgFile, &config.Config{
		Chezmoi: config.ChezmoiConfig{Enabled: config.BoolPtr(true)},
	})

	// Deny the cascade confirmation → abort.
	stdin := strings.NewReader("n\n")
	res := execCmdWith(t, home, stdin, withChezmoiManaged(true), "--config", cfgFile, "--manifest", manPath, "mcp", "remove", "context7")
	if res.err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
	}
	if strings.Contains(res.stderr, "chezmoi") {
		t.Errorf("expected no chezmoi output on abort, got: %s", res.stderr)
	}
}
