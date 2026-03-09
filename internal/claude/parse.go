package claude

import "github.com/alphaleonis/cctote/internal/manifest"

// pluginListEntry is the intermediate JSON shape returned by `claude plugin list --json`.
// Only parse.go needs to change if the CLI output format evolves.
type pluginListEntry struct {
	ID      string `json:"id"`
	Scope   string `json:"scope"`
	Enabled bool   `json:"enabled"`
}

func (e pluginListEntry) toPlugin() manifest.Plugin {
	return manifest.Plugin{
		ID:      e.ID,
		Scope:   e.Scope,
		Enabled: e.Enabled,
	}
}

// marketplaceListEntry is the intermediate JSON shape returned by
// `claude plugin marketplace list --json`.
type marketplaceListEntry struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Repo   string `json:"repo,omitempty"`
	URL    string `json:"url,omitempty"`
	Path   string `json:"path,omitempty"`
}

func (e marketplaceListEntry) toMarketplace() manifest.Marketplace {
	return manifest.Marketplace{
		Source: e.Source,
		Repo:   e.Repo,
		URL:    e.URL,
		Path:   e.Path,
	}
}
