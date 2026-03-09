# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## Unreleased

### Added
- CLI commands: `mcp`, `plugin`, `marketplace`, `profile`, `diff`, `config`, `version`
- TUI mode with sync dashboard for import/export/diff operations
- Manifest-based configuration sync for MCP servers, plugins, and marketplaces
- Profile support for managing multiple configuration sets
- Chezmoi integration for dotfile management
- Atomic file writes with file-based locking
- Per-package coverage enforcement via `check-coverage.sh`
- Enable/disable support for plugins and MCP servers
- Project-scope MCP server support (`.mcp.json`)
- Copy-as-JSON for MCP server definitions in TUI
