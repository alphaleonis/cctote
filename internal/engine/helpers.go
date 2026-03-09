package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// secretPatterns are substrings that, when found in env var key names
// (case-insensitive), suggest the value may be a secret. This is a
// best-effort heuristic — false positives (e.g. "TOKENIZER" matching
// "TOKEN") are acceptable since the warning is informational only.
var secretPatterns = []string{
	"API_KEY", "APIKEY",
	"SECRET",
	"TOKEN",
	"PASSWORD", "PASSWD",
	"CREDENTIAL",
	"PRIVATE_KEY",
	"ACCESS_KEY",
}

// MarketplaceFromPluginID extracts the marketplace name from a plugin ID
// in the format "pluginId@marketplaceName". Returns "" if no marketplace.
func MarketplaceFromPluginID(id string) string {
	if parts := strings.SplitN(id, "@", 2); len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// WarnProjectEnvVars emits an OnWarn if any MCP servers being applied to a
// project contain env vars. This alerts the user that secrets may be written
// to a project directory that could be version-controlled.
func WarnProjectEnvVars(servers map[string]manifest.MCPServer, names []string, hooks Hooks) {
	for _, name := range names {
		srv, ok := servers[name]
		if ok && len(srv.Env) > 0 {
			hooks.OnWarn("MCP servers with env vars are being written to .mcp.json — ensure this file is in .gitignore if it contains secrets")
			return // warn once
		}
	}
}

// WarnSecretEnvVars emits an informational warning via hooks.OnWarn when any
// MCP servers in the given name list have env var keys that look like secrets
// (e.g. API_KEY, TOKEN, SECRET). This is an informational warning only.
func WarnSecretEnvVars(servers map[string]manifest.MCPServer, names []string, hooks Hooks) {
	var hits []string
	for _, name := range names {
		srv, ok := servers[name]
		if !ok {
			continue
		}
		for envKey := range srv.Env {
			if looksLikeSecret(envKey) {
				hits = append(hits, fmt.Sprintf("%s (env %s)", name, envKey))
			}
		}
	}
	if len(hits) == 0 {
		return
	}
	sort.Strings(hits)
	hooks.OnWarn("importing MCP servers with env vars that may contain secrets: " + strings.Join(hits, ", "))
}

// looksLikeSecret returns true if the env var key name contains a substring
// commonly associated with secret values.
func looksLikeSecret(key string) bool {
	upper := strings.ToUpper(key)
	for _, pat := range secretPatterns {
		if strings.Contains(upper, pat) {
			return true
		}
	}
	return false
}
