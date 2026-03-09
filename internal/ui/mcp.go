package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// FormatMCPSummary returns a short human-readable summary of an MCP server's
// key fields: transport type, command+args (or URL), and env var keys.
//
// Values of env vars are deliberately never shown because CLI output may appear
// in terminal scrollback, CI logs, or screen shares. The TUI (tui/detail.go)
// takes a different approach with masked values and an explicit reveal toggle.
//
// Lines are returned without leading indentation — callers control alignment.
func FormatMCPSummary(srv manifest.MCPServer) []string {
	var lines []string

	typ := srv.Type
	if typ == "" {
		typ = "stdio"
	}

	switch typ {
	case "stdio":
		cmd := srv.Command
		if len(srv.Args) > 0 {
			cmd += " " + strings.Join(srv.Args, " ")
		}
		if cmd != "" {
			lines = append(lines, fmt.Sprintf("Command: %s", cmd))
		}
	default:
		if srv.URL != "" {
			lines = append(lines, fmt.Sprintf("URL:     %s", srv.URL))
		}
	}

	if len(srv.Env) > 0 {
		keys := make([]string, 0, len(srv.Env))
		for k := range srv.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines = append(lines, fmt.Sprintf("Env:     %s", strings.Join(keys, ", ")))
	}

	return lines
}
