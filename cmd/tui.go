package cmd

import (
	"github.com/alphaleonis/cctote/internal/config"
	"github.com/alphaleonis/cctote/internal/tui"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

func (a *App) runTUI(cmd *cobra.Command, profileName string) error {
	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}
	cfgPath, err := a.resolveConfigPath()
	if err != nil {
		return err
	}

	// Check chezmoi managed status once at launch (expensive CLI call).
	chezmoiManaged := false
	if a.appConfig != nil && config.BoolVal(a.appConfig.Chezmoi.Enabled) {
		mode := config.AutoReAddMode(a.appConfig.Chezmoi.AutoReAdd)
		if mode != config.AutoReAddNever {
			chezmoiManaged = a.chezmoiManaged(cmd.Context(), manPath)
		}
	}

	dirty, err := tui.Run(tui.Options{
		ManifestPath:   manPath,
		ConfigPath:     cfgPath,
		Profile:        profileName,
		ChezmoiManaged: chezmoiManaged,
		ChezmoiReAdd:   a.chezmoiReAdd,
	})
	// Only run post-TUI chezmoi for "never" mode (ask/always handled inside TUI).
	// Recompute mode from config in case the user changed it during the session.
	if dirty {
		effectiveMode := config.AutoReAddNever
		if chezmoiManaged {
			cfg, loadErr := config.Load(cfgPath)
			if loadErr == nil && cfg != nil {
				a.appConfig = cfg // Refresh so notifyChezmoi uses current settings.
				if config.BoolVal(cfg.Chezmoi.Enabled) {
					effectiveMode = config.AutoReAddMode(cfg.Chezmoi.AutoReAdd)
				}
			}
		}
		if effectiveMode == config.AutoReAddNever && chezmoiManaged {
			w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
			a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)
		}
	}
	return err
}

func (a *App) addTuiCommands() {
	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive terminal UI",
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileName, _ := cmd.Flags().GetString("profile")
			return a.runTUI(cmd, profileName)
		},
	}
	tuiCmd.Flags().String("profile", "", "pre-select a profile in the left panel")
	a.root.AddCommand(tuiCmd)
}
