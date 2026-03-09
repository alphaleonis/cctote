package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

func (a *App) addProfileCommands() {
	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configuration profiles",
	}

	profileCreateCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new profile from the current Claude Code configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runProfileSnapshot(cmd, args[0], false)
		},
	}

	profileApplyCmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Apply a profile to Claude Code",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runProfileApply,
	}

	profileUpdateCmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Overwrite a profile with the current Claude Code configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runProfileSnapshot(cmd, args[0], true)
		},
	}

	profileDeleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a profile from the manifest",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runProfileDelete,
	}

	profileRenameCmd := &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a profile",
		Args:  cobra.ExactArgs(2),
		RunE:  a.runProfileRename,
	}

	profileListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		RunE:  a.runProfileList,
	}

	profileApplyCmd.Flags().Bool("strict", false, "remove extensions not in the profile")
	profileApplyCmd.Flags().Bool("dry-run", false, "show what would change without modifying anything")
	profileApplyCmd.Flags().Bool("overwrite", false, "overwrite differing MCP servers without confirmation")
	profileApplyCmd.Flags().Bool("no-overwrite", false, "skip differing MCP servers without confirmation")
	addScopeFlag(profileApplyCmd)

	profileCmd.AddCommand(profileCreateCmd, profileApplyCmd, profileUpdateCmd, profileDeleteCmd, profileRenameCmd, profileListCmd)
	a.root.AddCommand(profileCmd)
}

// --- profile list ---

func (a *App) runProfileList(cmd *cobra.Command, _ []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	m, _, err := manifest.LoadOrCreate(manPath)
	if err != nil {
		return err
	}

	if len(m.Profiles) == 0 {
		if a.jsonOutput {
			return writeJSON(cmd, map[string]manifest.Profile{})
		}
		w.Info("No profiles in manifest.")
		return nil
	}

	if a.jsonOutput {
		return writeJSON(cmd, m.Profiles)
	}

	names := sortedKeys(m.Profiles)
	rows := make([][]string, 0, len(names))
	for _, name := range names {
		p := m.Profiles[name]
		rows = append(rows, []string{
			name,
			fmt.Sprintf("%d", len(p.MCPServers)),
			fmt.Sprintf("%d", len(p.Plugins)),
		})
	}
	w.Table(cmd.OutOrStdout(), []string{"NAME", "MCP SERVERS", "PLUGINS"}, rows)
	return nil
}

// --- profile delete ---

func (a *App) runProfileDelete(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
	name := args[0]

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		if _, ok := m.Profiles[name]; !ok {
			return fmt.Errorf("profile %q not found", name)
		}
		delete(m.Profiles, name)
		// Nil the map so json:"omitempty" omits the "profiles" key entirely,
		// keeping the manifest clean when no profiles exist.
		if len(m.Profiles) == 0 {
			m.Profiles = nil
		}
		return nil
	}); err != nil {
		return err
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{"deleted": name})
	}

	w.Success("Deleted profile %q", name)
	return nil
}

// --- profile rename ---

func (a *App) runProfileRename(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
	oldName, newName := args[0], args[1]

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		profile, ok := m.Profiles[oldName]
		if !ok {
			return fmt.Errorf("profile %q not found", oldName)
		}
		if _, exists := m.Profiles[newName]; exists {
			return fmt.Errorf("profile %q already exists", newName)
		}
		m.Profiles[newName] = profile
		delete(m.Profiles, oldName)
		return nil
	}); err != nil {
		return err
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{"old": oldName, "new": newName})
	}

	w.Success("Renamed profile %q to %q", oldName, newName)
	return nil
}

// --- profile create / update (shared snapshot helper) ---

func (a *App) runProfileSnapshot(cmd *cobra.Command, name string, mustExist bool) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	if err := a.ensureClaudeAvailable(); err != nil {
		return err
	}

	// Read MCP servers from ~/.claude.json.
	claudePath, err := mcp.DefaultPath()
	if err != nil {
		return err
	}
	installed, err := mcp.ReadMcpServers(claudePath)
	if err != nil {
		return fmt.Errorf("reading MCP servers: %w", err)
	}

	// Read plugins from Claude CLI.
	client := a.newClaudeClient()
	ctx := cmd.Context()
	plugins, err := client.ListPlugins(ctx)
	if err != nil {
		return err
	}

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	// Load existing marketplaces so lazyLoadMarketplaces can skip the CLI call
	// when all referenced marketplaces are already in the manifest.
	var existingMkts map[string]manifest.Marketplace
	if m, loadErr := manifest.Load(manPath); loadErr == nil {
		existingMkts = m.Marketplaces
	}
	claudeMarketplaces, err := lazyLoadMarketplaces(ctx, client, existingMkts, plugins)
	if err != nil {
		return err
	}

	hooks := &cliHooks{cmd: cmd, w: w, section: "marketplace", force: a.forceFlag, cascadeMsg: "Marketplace %q is required by:"}
	result, err := engine.SnapshotProfile(manPath, name, mustExist, installed, plugins, claudeMarketplaces, hooks)
	if err != nil {
		return err
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)
	return a.reportSnapshotResult(cmd, w, result, name, mustExist, sortedKeys(installed))
}

// reportSnapshotResult formats and prints the profile create/update outcome.
func (a *App) reportSnapshotResult(cmd *cobra.Command, w *ui.Writer, result *engine.Result, name string, mustExist bool, mcpNames []string) error {
	// Derive exported plugin IDs and per-section counts from result actions.
	var (
		mcpAdded, mcpUpdated       int
		pluginAdded, pluginUpdated int
		pluginIDs                  []string
		cascaded                   []string
	)
	// NOTE: ApplyPluginUpserts emits ActionAdded or ActionUpdated for EVERY
	// exportable plugin (there is no skip/unchanged path), so pluginIDs
	// captures the complete set.
	for _, action := range result.Actions {
		switch {
		case action.Section == engine.SectionMCP && action.Action == engine.ActionAdded:
			mcpAdded++
		case action.Section == engine.SectionMCP && action.Action == engine.ActionUpdated:
			mcpUpdated++
		case action.Section == engine.SectionPlugin && action.Action == engine.ActionAdded:
			pluginAdded++
			pluginIDs = append(pluginIDs, action.Name)
		case action.Section == engine.SectionPlugin && action.Action == engine.ActionUpdated:
			pluginUpdated++
			pluginIDs = append(pluginIDs, action.Name)
		// Marketplace actions appear as ActionCascaded — there is no separate
		// SectionMarketplace Added/Updated path in SnapshotProfile.
		case action.Action == engine.ActionCascaded:
			cascaded = append(cascaded, action.Name)
		}
	}
	sort.Strings(pluginIDs)
	sort.Strings(cascaded)

	if a.jsonOutput {
		out := map[string]any{
			"profile": name,
			"profileContents": map[string]any{
				"mcpServers": mcpNames,
				"plugins":    pluginIDs,
			},
			"mcpServers": map[string]any{"added": mcpAdded, "updated": mcpUpdated},
			"plugins":    map[string]any{"added": pluginAdded, "updated": pluginUpdated},
		}
		if len(cascaded) > 0 {
			out["autoExportedMarketplaces"] = cascaded
		}
		return writeJSON(cmd, out)
	}

	verb := "Created"
	if mustExist {
		verb = "Updated"
	}
	w.Success("%s profile %q: %d MCP server(s), %d plugin(s)", verb, name, len(mcpNames), len(pluginIDs))
	if mcpAdded+mcpUpdated > 0 {
		w.Info("MCP servers exported: %d added, %d updated", mcpAdded, mcpUpdated)
	}
	if pluginAdded+pluginUpdated > 0 {
		w.Info("Plugins exported: %d added, %d updated", pluginAdded, pluginUpdated)
	}
	if len(cascaded) > 0 {
		w.Info("Auto-exported marketplace(s): %s", strings.Join(cascaded, ", "))
	}
	return nil
}

// --- profile apply ---

// applyOpts holds validated flags for profile apply.
type applyOpts struct {
	name        string
	scope       string
	overwrite   bool
	noOverwrite bool
	strict      bool
	dryRun      bool
	projPath    string // set when scope==scopeProject
}

func (a *App) parseApplyOpts(cmd *cobra.Command, name string) (applyOpts, error) {
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	noOverwrite, _ := cmd.Flags().GetBool("no-overwrite")
	strict, _ := cmd.Flags().GetBool("strict")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	scope, err := getScope(cmd)
	if err != nil {
		return applyOpts{}, err
	}

	if a.forceFlag {
		overwrite = true
	}
	if overwrite && noOverwrite {
		return applyOpts{}, fmt.Errorf("--overwrite and --no-overwrite are mutually exclusive (--force implies --overwrite)")
	}
	if strict && noOverwrite {
		return applyOpts{}, fmt.Errorf("--strict and --no-overwrite are mutually exclusive (--strict requires full convergence)")
	}

	opts := applyOpts{
		name:        name,
		scope:       scope,
		overwrite:   overwrite,
		noOverwrite: noOverwrite,
		strict:      strict,
		dryRun:      dryRun,
	}
	if scope == scopeProject {
		opts.projPath = a.resolveProjectMcpPath()
	}
	return opts, nil
}

// readCurrentMCP reads the current MCP server state based on scope.
func readCurrentMCP(opts applyOpts) (map[string]manifest.MCPServer, error) {
	if opts.scope == scopeProject {
		servers, err := mcp.ReadProjectMcpServers(opts.projPath)
		if err != nil {
			return nil, fmt.Errorf("reading project MCP servers: %w", err)
		}
		return servers, nil
	}
	claudePath, err := mcp.DefaultPath()
	if err != nil {
		return nil, err
	}
	servers, err := mcp.ReadMcpServers(claudePath)
	if err != nil {
		return nil, fmt.Errorf("reading MCP servers: %w", err)
	}
	return servers, nil
}

// resolveApplyConflicts prompts the user for each conflicting MCP server
// and returns the list of servers the user chose to overwrite.
// The desired map provides the incoming server definitions for display.
func resolveApplyConflicts(cmd *cobra.Command, w *ui.Writer, conflicts []string, desired map[string]manifest.MCPServer, opts applyOpts) ([]string, error) {
	var overwriteNames []string
	for _, srvName := range conflicts {
		if opts.overwrite {
			overwriteNames = append(overwriteNames, srvName)
		} else if opts.noOverwrite {
			// skip
		} else {
			prompt := fmt.Sprintf("MCP server %q differs — overwrite?", srvName)
			if srv, ok := desired[srvName]; ok {
				for _, line := range ui.FormatMCPSummary(srv) {
					w.Faint("  %s", line)
				}
			}
			yes, err := ui.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), prompt, false)
			if err != nil {
				return nil, err
			}
			if yes {
				overwriteNames = append(overwriteNames, srvName)
			}
		}
	}
	return overwriteNames, nil
}

// confirmStrictRemovals prompts the user to confirm --strict removals.
// Returns true if the user aborted. When force is true, confirmations are skipped.
// currentMCP provides server definitions for displaying details alongside names.
func confirmStrictRemovals(cmd *cobra.Command, w *ui.Writer, mcpPlan, pluginPlan *engine.ImportPlan, currentMCP map[string]manifest.MCPServer, force bool) (bool, error) {
	if len(mcpPlan.Remove) > 0 {
		w.Warn("The following MCP servers will be removed (--strict):")
		for _, name := range mcpPlan.Remove {
			w.List([]string{name})
			if srv, ok := currentMCP[name]; ok {
				for _, line := range ui.FormatMCPSummary(srv) {
					w.Faint("      %s", line)
				}
			}
		}
		if !force {
			yes, err := ui.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), "Proceed?", false)
			if err != nil {
				return false, err
			}
			if !yes {
				w.Abort()
				return true, nil
			}
		}
	}
	if len(pluginPlan.Remove) > 0 {
		w.Warn("The following plugins will be uninstalled (--strict):")
		w.List(pluginPlan.Remove)
		if !force {
			yes, err := ui.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), "Proceed?", false)
			if err != nil {
				return false, err
			}
			if !yes {
				w.Abort()
				return true, nil
			}
		}
	}
	return false, nil
}

// applyMCPChanges writes MCP server changes to the appropriate config file
// based on scope (user ~/.claude.json or project .mcp.json).
func applyMCPChanges(cmd *cobra.Command, w *ui.Writer, opts applyOpts, mcpImport map[string]manifest.MCPServer, mcpPlan *engine.ImportPlan, overwriteNames []string, force bool) error {
	if opts.scope == scopeProject {
		mcpHooks := &cliHooks{cmd: cmd, w: w, section: "MCP server", force: force}
		engine.WarnProjectEnvVars(mcpImport, append(mcpPlan.Add, overwriteNames...), mcpHooks)
		return engine.ApplyMCPImportToProject(opts.projPath, mcpImport, mcpPlan.Add, overwriteNames, mcpPlan.Remove)
	}
	claudePath, err := mcp.DefaultPath()
	if err != nil {
		return err
	}
	return engine.ApplyMCPImport(claudePath, mcpImport, mcpPlan.Add, overwriteNames, mcpPlan.Remove)
}

// reportApplyResult formats and prints the profile apply outcome.
func (a *App) reportApplyResult(cmd *cobra.Command, w *ui.Writer, mcpPlan *engine.ImportPlan, overwriteNames []string, pluginResult *engine.PluginImportResult) error {
	mcpSkipped := len(mcpPlan.Conflict) - len(overwriteNames)
	if a.jsonOutput {
		out := map[string]any{
			"mcp": map[string]any{
				"added":       len(mcpPlan.Add),
				"overwritten": len(overwriteNames),
				"skipped":     len(mcpPlan.Skip) + mcpSkipped,
				"removed":     len(mcpPlan.Remove),
			},
			"plugins": map[string]any{
				"installed":   pluginResult.Installed,
				"reconciled":  pluginResult.Reconciled,
				"skipped":     pluginResult.Skipped,
				"uninstalled": pluginResult.Uninstalled,
			},
		}
		if len(pluginResult.Errors) > 0 {
			errStrs := make([]string, len(pluginResult.Errors))
			for i, e := range pluginResult.Errors {
				errStrs[i] = e.Error()
			}
			out["errors"] = errStrs
		}
		if err := writeJSON(cmd, out); err != nil {
			return err
		}
		return pluginResult.Err()
	}

	w.Success("MCP: %d added, %d overwritten, %d unchanged, %d removed",
		len(mcpPlan.Add), len(overwriteNames), len(mcpPlan.Skip)+mcpSkipped, len(mcpPlan.Remove))
	w.Success("Plugins: %d installed, %d reconciled, %d unchanged, %d uninstalled",
		pluginResult.Installed, pluginResult.Reconciled, pluginResult.Skipped, pluginResult.Uninstalled)
	if len(pluginResult.Errors) > 0 {
		for _, e := range pluginResult.Errors {
			w.Error("%s", e)
		}
		return pluginResult.Err()
	}
	return nil
}

func (a *App) runProfileApply(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
	force := a.forceFlag // capture once; threaded to extracted helpers

	opts, err := a.parseApplyOpts(cmd, args[0])
	if err != nil {
		return err
	}

	if err := a.ensureClaudeAvailable(); err != nil {
		return err
	}

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}
	m, err := manifest.Load(manPath)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	mcpImport, pluginImport, err := resolveProfile(m, opts.name)
	if err != nil {
		return err
	}

	// --- Classify MCP changes ---
	currentMCP, err := readCurrentMCP(opts)
	if err != nil {
		return err
	}
	// --strict scopes removal to the PROFILE's references, not the full manifest.
	// A profile is a subset, so --strict here removes extensions not in this profile.
	mcpPlan := engine.ClassifyMCPImport(mcpImport, currentMCP, opts.strict)

	// --- Classify plugin changes ---
	client := a.newClaudeClient()
	ctx := cmd.Context()
	allPlugins, err := client.ListPlugins(ctx)
	if err != nil {
		return err
	}
	var currentPlugins []manifest.Plugin
	if opts.scope == scopeProject {
		for _, p := range allPlugins {
			if p.Scope == scopeProject {
				currentPlugins = append(currentPlugins, p)
			}
		}
	} else {
		// User scope: exclude project-scoped plugins so --strict doesn't
		// attempt to uninstall them at the wrong scope.
		for _, p := range allPlugins {
			if p.Scope != scopeProject {
				currentPlugins = append(currentPlugins, p)
			}
		}
	}
	pluginPlan := engine.ClassifyPluginImport(pluginImport, currentPlugins, opts.strict)

	// Marketplace prerequisite check for new plugins.
	if err := lazyCheckPluginPrereqs(ctx, client, pluginPlan.Add); err != nil {
		return err
	}

	// --- Dry-run ---
	if opts.dryRun {
		return a.printProfileApplyPlan(cmd, w, mcpPlan, pluginPlan)
	}

	// --- Interactive resolution ---
	overwriteNames, err := resolveApplyConflicts(cmd, w, mcpPlan.Conflict, mcpImport, opts)
	if err != nil {
		return err
	}

	// Warn about env vars that look like secrets in servers being added/overwritten.
	mcpHooksForWarn := &cliHooks{cmd: cmd, w: w, section: "MCP server", force: force}
	engine.WarnSecretEnvVars(mcpImport, append(mcpPlan.Add, overwriteNames...), mcpHooksForWarn)

	aborted, err := confirmStrictRemovals(cmd, w, mcpPlan, pluginPlan, currentMCP, force)
	if err != nil {
		return err
	}
	if aborted {
		return nil
	}

	// --- Apply ---
	if err := applyMCPChanges(cmd, w, opts, mcpImport, mcpPlan, overwriteNames, force); err != nil {
		return err
	}

	pluginScope := ""
	if opts.scope == scopeProject {
		pluginScope = scopeProject
	}
	pluginHooks := &cliHooks{cmd: cmd, w: w, section: "plugin", force: force}
	importPluginMap := engine.PluginMap(pluginImport)
	currentPluginMap := engine.PluginMap(currentPlugins)
	pluginResult := engine.ApplyPluginImport(ctx, client, pluginPlan, importPluginMap, currentPluginMap, pluginHooks, pluginScope)

	return a.reportApplyResult(cmd, w, mcpPlan, overwriteNames, pluginResult)
}

func (a *App) printProfileApplyPlan(cmd *cobra.Command, w *ui.Writer, mcpPlan, pluginPlan *engine.ImportPlan) error {
	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"mcp": map[string]any{
				"add":      mcpPlan.Add,
				"skip":     mcpPlan.Skip,
				"conflict": mcpPlan.Conflict,
				"remove":   mcpPlan.Remove,
			},
			"plugins": map[string]any{
				"install":   pluginPlan.Add,
				"reconcile": pluginPlan.Conflict,
				"skip":      pluginPlan.Skip,
				"uninstall": pluginPlan.Remove,
			},
		})
	}

	hasMCP := len(mcpPlan.Add)+len(mcpPlan.Skip)+len(mcpPlan.Conflict)+len(mcpPlan.Remove) > 0
	hasPlugins := len(pluginPlan.Add)+len(pluginPlan.Conflict)+len(pluginPlan.Skip)+len(pluginPlan.Remove) > 0

	if hasMCP {
		w.Bold(cmd.ErrOrStderr(), "MCP Servers\n")
		w.DiffList(ui.DiffAdd, mcpPlan.Add)
		w.DiffList(ui.DiffSkip, mcpPlan.Skip)
		w.DiffList(ui.DiffConflict, mcpPlan.Conflict)
		w.DiffList(ui.DiffRemove, mcpPlan.Remove)
	}
	if hasPlugins {
		w.Bold(cmd.ErrOrStderr(), "Plugins\n")
		w.DiffList(ui.DiffAdd, pluginPlan.Add)
		w.DiffList(ui.DiffConflict, pluginPlan.Conflict)
		w.DiffList(ui.DiffSkip, pluginPlan.Skip)
		w.DiffList(ui.DiffRemove, pluginPlan.Remove)
	}
	if !hasMCP && !hasPlugins {
		w.NothingToDo()
	}
	return nil
}
