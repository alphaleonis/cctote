package tui

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
)

// stateResult bundles loaded state with non-fatal warnings.
type stateResult struct {
	state    *FullState
	warnings []string
}

func loadState(opts Options) tea.Cmd {
	return func() tea.Msg {
		res, err := doLoadState(opts)
		if err != nil {
			return StateLoadedMsg{Err: err}
		}
		return StateLoadedMsg{State: res.state, Warnings: res.warnings}
	}
}

func refreshState(opts Options) tea.Cmd {
	return func() tea.Msg {
		res, err := doLoadState(opts)
		if err != nil {
			return StateRefreshedMsg{Err: err}
		}
		return StateRefreshedMsg{State: res.state, Warnings: res.warnings}
	}
}

func doLoadState(opts Options) (*stateResult, error) {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return nil, err
	}

	man, _, err := manifest.LoadOrCreate(manPath)
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	var warnings []string

	claudePath, err := mcp.DefaultPath()
	if err != nil {
		return nil, err
	}
	claudeMCP, err := mcp.ReadMcpServers(claudePath)
	if err != nil {
		return nil, fmt.Errorf("reading Claude Code MCP config: %w", err)
	}

	ctx := context.Background()
	client := opts.NewClient()

	// Plugin and marketplace listing requires the Claude CLI binary, which may
	// not be installed (e.g., headless machines, CI). These are non-fatal: the
	// TUI degrades gracefully to MCP-only mode. In contrast, manifest and MCP
	// config are local files that must be readable for the TUI to function.
	claudePlugins, err := client.ListPlugins(ctx)
	if err != nil {
		claudePlugins = nil
		warnings = append(warnings, fmt.Sprintf("Could not list plugins: %v", err))
	}

	claudeMarketplaces, err := client.ListMarketplaces(ctx)
	if err != nil {
		claudeMarketplaces = nil
		warnings = append(warnings, fmt.Sprintf("Could not list marketplaces: %v", err))
	}

	// Detect project-level config: use working directory as project root,
	// read .mcp.json, filter project-scoped plugins. Claude Code uses cwd
	// (not git root) for project scoping — the projects key in ~/.claude.json
	// is keyed by launch directory.
	var projectMCP map[string]manifest.MCPServer
	var projectPlugins []manifest.Plugin
	projectRoot := opts.WorkDir

	if projectRoot != "" {
		projectMCP, err = mcp.ReadProjectMcpServers(filepath.Join(projectRoot, ".mcp.json"))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not read .mcp.json: %v", err))
		}
		// Filter project-scoped plugins independently of .mcp.json success —
		// plugins are loaded from the Claude CLI, not from .mcp.json.
		for _, p := range claudePlugins {
			if p.Scope == pluginScopeProject {
				projectPlugins = append(projectPlugins, p)
			}
		}
	}

	return &stateResult{
		state: &FullState{
			Manifest:       man,
			MCPInstalled:   claudeMCP,
			PlugInstalled:  claudePlugins,
			MktInstalled:   claudeMarketplaces,
			ProjectMCP:     projectMCP,
			ProjectPlugins: projectPlugins,
			ProjectRoot:    projectRoot,
		},
		warnings: warnings,
	}, nil
}
