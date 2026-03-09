# Alphaleonis Tote Bag for Claude Code

[![CI](https://github.com/alphaleonis/cctote/actions/workflows/ci.yml/badge.svg)](https://github.com/alphaleonis/cctote/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/alphaleonis/cctote)](https://go.dev/)
[![License](https://img.shields.io/github/license/alphaleonis/cctote)](LICENSE)

**Alphaleonis Tote Bag for Claude Code** — sync your Claude Code configuration (MCP servers, plugins, marketplaces) across machines through a portable manifest file.

cctote exports your Claude Code setup into a single declarative manifest, then imports it on any other machine. Pair it with [chezmoi](https://www.chezmoi.io/) and your config follows you everywhere.

> [!WARNING]
> cctote directly reads and writes Claude Code's internal configuration files (e.g. `~/.claude.json`), which are undocumented implementation details that may change at any time. It also invokes the Claude Code CLI for plugin and marketplace operations, which could equally change without notice. A Claude Code update could break cctote until it is updated to match. Use at your own risk.

## Why?

Claude Code stores MCP servers in `~/.claude.json`, plugins via its own CLI, and marketplace sources separately. None of this travels between machines. cctote solves this by:

- **Exporting** your live Claude Code config into a versioned manifest (`manifest.json`)
- **Importing** that manifest on a new machine to converge Claude Code toward the desired state
- **Profiles** for switching between different configurations (e.g. work vs personal)
- **Diffing** manifest vs live state to see what's out of sync
- **chezmoi integration** for automatic dotfile management

## Installation

### From source

```bash
go install github.com/alphaleonis/cctote@latest
```

### From releases

Download pre-built binaries from [GitHub Releases](https://github.com/alphaleonis/cctote/releases) (Linux and Windows, amd64/arm64).

## Quick Start

```bash
# Export all MCP servers from Claude Code to the manifest
cctote mcp export

# Export all plugins (auto-exports required marketplaces)
cctote plugin export

# On another machine: import everything
cctote mcp import
cctote plugin import

# See what's different between manifest and Claude Code
cctote diff

# Create a profile from your current setup
cctote profile create work

# Apply a profile on another machine
cctote profile apply work
```

## Command Reference

```
cctote
├── mcp
│   ├── export [names...]       Export MCP servers to manifest
│   ├── import [names...]       Import MCP servers from manifest
│   ├── remove <name>           Remove an MCP server from manifest
│   └── list                    List MCP servers
├── plugin
│   ├── export [ids...]         Export plugins to manifest
│   ├── import [ids...]         Import plugins from manifest
│   ├── remove <id>             Remove a plugin from manifest
│   └── list                    List plugins
├── marketplace
│   ├── export [names...]       Export marketplace sources to manifest
│   ├── import [names...]       Import marketplace sources from manifest
│   ├── remove <name>           Remove a marketplace from manifest
│   └── list                    List marketplaces
├── profile
│   ├── create <name>           Snapshot current config as a profile
│   ├── apply <name>            Apply a profile to Claude Code
│   ├── update <name>           Re-snapshot an existing profile
│   ├── delete <name>           Delete a profile
│   ├── rename <old> <new>      Rename a profile
│   └── list                    List profiles
├── diff                        Show manifest vs Claude Code differences
├── config
│   ├── get <key>               Get a config value
│   ├── set <key> <value>       Set a config value
│   ├── reset <key>             Reset to default
│   └── list                    List all config keys
├── tui                         Interactive terminal dashboard
└── version                     Print version
```

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config <path>` | | Path to cctote config file |
| `--manifest <path>` | | Path to manifest file (overrides config) |
| `--json` | | Structured JSON output |
| `--force` | `-f` | Skip all confirmation prompts |

### Import Flags

Import commands (`mcp import`, `plugin import`, `marketplace import`, `profile apply`) support:

| Flag | Description |
|------|-------------|
| `--strict` | Remove items from Claude Code not in the manifest/profile |
| `--dry-run` | Preview changes without applying |
| `--overwrite` | Overwrite differing items without prompting |
| `--no-overwrite` | Skip differing items without prompting |
| `--scope` / `-s` | `user` (default) or `project` scope |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error |
| 2 | `diff` found differences |

## Scopes

cctote supports two configuration scopes:

| Scope | MCP servers | Plugins |
|-------|-------------|---------|
| `user` (default) | `~/.claude.json` | User-scoped |
| `project` | `.mcp.json` in cwd | Project-scoped |

```bash
# Export project-level MCP servers
cctote mcp export --scope project

# List installed project plugins
cctote plugin list --installed --scope project
```

## TUI Mode

`cctote tui` launches a dual-pane terminal dashboard for interactive browsing and syncing.

```
┌─ Manifest ─────────────────────┐┌─ Claude Code ──────────────────────┐
│ MCP Servers                    ││ MCP Servers                        │
│   filesystem    stdio          ││   filesystem    stdio              │
│   github        http     ≠     ││   github        http     ≠         │
│   slack         stdio          ││   postgres      sse                │
│                                ││                                    │
│ Plugins                        ││ Plugins                            │
│   my-plugin     global      on ││   my-plugin     global       on    │
└────────────────────────────────┘└────────────────────────────────────┘
```

Each pane can show the manifest, a profile, Claude Code's live state, or the project config. Copy items between panes with keyboard shortcuts.

## Chezmoi Integration

cctote integrates with [chezmoi](https://www.chezmoi.io/) for automatic dotfile syncing:

```bash
# Enable chezmoi integration
cctote config set chezmoi.enabled true

# Auto-run `chezmoi re-add` after manifest changes
cctote config set chezmoi.auto_re_add true
```

When enabled, cctote notifies chezmoi after every manifest change so your config stays in sync across machines.

## Configuration

cctote stores its own config at `~/.config/cctote/cctote.toml` (respects `$XDG_CONFIG_HOME`).

| Key | Default | Description |
|-----|---------|-------------|
| `manifest_path` | `~/.config/cctote/manifest.json` | Path to manifest file |
| `chezmoi.enabled` | `false` | Notify chezmoi after manifest changes |
| `chezmoi.auto_re_add` | `false` | Auto-run `chezmoi re-add` |

## The Manifest

The manifest is a declarative JSON file describing your desired Claude Code configuration:

```json
{
  "version": 1,
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-fs"]
    },
    "api-server": {
      "type": "http",
      "url": "https://api.example.com/mcp"
    }
  },
  "plugins": [
    { "id": "context7@claude-plugins-official", "scope": "user", "enabled": true }
  ],
  "marketplaces": {
    "claude-plugins-official": {
      "source": "github",
      "repo": "anthropics/claude-plugins-official"
    }
  },
  "profiles": {
    "work": {
      "mcpServers": ["filesystem", "api-server"],
      "plugins": [{ "id": "context7@claude-plugins-official" }]
    }
  }
}
```

Profiles reference items by name/ID — they don't duplicate data. The manifest carries no machine-specific runtime state, making it fully portable.

## JSON Output

All commands support `--json` for scripted usage:

```bash
# Pipe diff results to jq
cctote diff --json | jq '.onlyInManifest[]'

# Check if everything is in sync
if cctote diff --json > /dev/null 2>&1; then
  echo "In sync"
else
  echo "Drift detected"
fi
```

## Requirements

- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) (`claude`) on `$PATH` — required for plugin and marketplace operations
- Go 1.25+ (for building from source)

## Development

```bash
mise run build          # Compile binary
mise run test           # Run all tests
mise run lint           # golangci-lint
mise run fmt            # Format code
mise run coverage       # Enforce per-package coverage thresholds
```

## License

See [LICENSE](LICENSE) for details.

## Disclaimer

*cctote is not affiliated with or endorsed by Anthropic, PBC.*

*The majority of this codebase was generated using [Claude Code](https://claude.ai/code).*
