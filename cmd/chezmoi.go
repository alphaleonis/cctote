package cmd

import (
	"context"
	"io"

	"github.com/alphaleonis/cctote/internal/cliutil"
	"github.com/alphaleonis/cctote/internal/config"
	"github.com/alphaleonis/cctote/internal/ui"
)

// defaultChezmoiReAdd is the production chezmoi re-add implementation.
func defaultChezmoiReAdd(ctx context.Context, manifestPath string) error {
	r := &cliutil.ExecRunner{Command: "chezmoi"}
	_, err := r.Run(ctx, "re-add", manifestPath)
	return err
}

// defaultChezmoiManaged is the production chezmoi managed check.
func defaultChezmoiManaged(ctx context.Context, manifestPath string) bool {
	r := &cliutil.ExecRunner{Command: "chezmoi"}
	_, err := r.Run(ctx, "managed", "--include=files", "--path-style=absolute", manifestPath)
	return err == nil
}

// notifyChezmoi prints a chezmoi reminder, prompts the user, or auto-runs
// `chezmoi re-add` after a command that modified the manifest.
// The mode is controlled by chezmoi.auto_re_add: "never" (default) prints a
// reminder, "ask" prompts via stdin, "always" runs automatically.
// Errors are warnings — the primary operation already succeeded.
func (a *App) notifyChezmoi(ctx context.Context, w *ui.Writer, stdin io.Reader, manifestPath string) {
	if a.appConfig == nil || !config.BoolVal(a.appConfig.Chezmoi.Enabled) {
		return
	}

	if !a.chezmoiManaged(ctx, manifestPath) {
		return
	}

	mode := config.AutoReAddMode(a.appConfig.Chezmoi.AutoReAdd)

	switch mode {
	case config.AutoReAddAlways:
		a.runChezmoiReAdd(ctx, w, manifestPath)
	case config.AutoReAddAsk:
		ok, err := ui.Confirm(stdin, w.Writer(), "Run chezmoi re-add?", false)
		if err != nil {
			w.Warn("reading confirmation: %s", err)
			return
		}
		if ok {
			a.runChezmoiReAdd(ctx, w, manifestPath)
		}
	default: // "never"
		w.Info("Run 'chezmoi re-add %s' to sync changes", manifestPath)
	}
}

// runChezmoiReAdd executes chezmoi re-add with progress output.
func (a *App) runChezmoiReAdd(ctx context.Context, w *ui.Writer, manifestPath string) {
	w.Info("Running chezmoi re-add...")
	if err := a.chezmoiReAdd(ctx, manifestPath); err != nil {
		w.Warn("chezmoi re-add failed: %s", err)
		return
	}
	w.Info("Ran chezmoi re-add for %s", manifestPath)
}
