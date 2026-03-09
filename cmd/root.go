package cmd

import (
	"context"

	"github.com/alphaleonis/cctote/internal/claude"
	"github.com/alphaleonis/cctote/internal/config"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/spf13/cobra"
)

// App holds shared CLI state. All command handlers are methods on App.
type App struct {
	cfgPath      string
	manifestPath string
	jsonOutput   bool
	forceFlag    bool
	appConfig    *config.Config
	root         *cobra.Command

	// Testability seams — initialized to production defaults in NewApp.
	// Tests override these on the App instance before Execute().
	newClaudeClient       func() *claude.Client
	ensureClaudeAvailable func() error
	resolveProjectMcpPath func() string
	chezmoiReAdd          func(ctx context.Context, manifestPath string) error
	chezmoiManaged        func(ctx context.Context, manifestPath string) bool
}

// NewApp creates a fresh command tree with all subcommands attached.
func NewApp(version string) *App {
	a := &App{
		newClaudeClient:       claude.NewExecClient,
		ensureClaudeAvailable: claude.EnsureAvailable,
		resolveProjectMcpPath: func() string { return ".mcp.json" },
		chezmoiReAdd:          defaultChezmoiReAdd,
		chezmoiManaged:        defaultChezmoiManaged,
	}

	a.root = &cobra.Command{
		Use:           "cctote",
		Short:         "Alphaleonis Tote Bag for Claude Code",
		Long:          "Alphaleonis Tote Bag for Claude Code — sync your Claude Code configuration across machines.\n\nRunning without a subcommand launches the interactive TUI.\nUse --help to see available commands.",
		Version:       version,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return a.loadConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTUI(cmd, "")
		},
	}

	a.root.PersistentFlags().StringVar(&a.cfgPath, "config", "", "path to config file (default: ~/.config/cctote/cctote.toml)")
	a.root.PersistentFlags().StringVar(&a.manifestPath, "manifest", "", "path to manifest file; overrides config")
	a.root.PersistentFlags().BoolVar(&a.jsonOutput, "json", false, "output in JSON format")
	a.root.PersistentFlags().BoolVarP(&a.forceFlag, "force", "f", false, "skip confirmation prompts (implies --overwrite for import)")

	a.addVersionCommands()
	a.addConfigCommands()
	a.addDiffCommands()
	a.addTuiCommands()
	a.addMcpCommands()
	a.addPluginCommands()
	a.addMarketplaceCommands()
	a.addProfileCommands()

	return a
}

// Execute runs the root command.
func (a *App) Execute() error {
	return a.root.Execute()
}

func (a *App) loadConfig() error {
	path := a.cfgPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return err
		}
		path = p
	}

	c, err := config.Load(path)
	if err != nil {
		return err
	}
	a.appConfig = c
	return nil
}

func (a *App) resolveManifestPath() (string, error) {
	if a.manifestPath != "" {
		return a.manifestPath, nil
	}
	if a.appConfig != nil && a.appConfig.ManifestPath != "" {
		return a.appConfig.ManifestPath, nil
	}
	return manifest.DefaultPath()
}
