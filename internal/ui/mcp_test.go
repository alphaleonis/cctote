package ui

import (
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestFormatMCPSummary(t *testing.T) {
	tests := []struct {
		name string
		srv  manifest.MCPServer
		want []string
	}{
		{
			name: "stdio with command and args",
			srv: manifest.MCPServer{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
			},
			want: []string{"Command: npx -y @modelcontextprotocol/server-everything"},
		},
		{
			name: "stdio with env keys",
			srv: manifest.MCPServer{
				Command: "node",
				Args:    []string{"server.js"},
				Env:     map[string]string{"API_KEY": "secret123", "PORT": "3000"},
			},
			want: []string{
				"Command: node server.js",
				"Env:     API_KEY, PORT",
			},
		},
		{
			name: "http with URL",
			srv: manifest.MCPServer{
				Type: "sse",
				URL:  "https://example.com/mcp",
			},
			want: []string{"URL:     https://example.com/mcp"},
		},
		{
			name: "empty server",
			srv:  manifest.MCPServer{},
			want: nil,
		},
		{
			name: "explicit stdio type",
			srv: manifest.MCPServer{
				Type:    "stdio",
				Command: "python",
			},
			want: []string{"Command: python"},
		},
		{
			name: "http with URL and headers env",
			srv: manifest.MCPServer{
				Type: "http",
				URL:  "https://api.example.com",
				Env:  map[string]string{"AUTH_TOKEN": "tok"},
			},
			want: []string{
				"URL:     https://api.example.com",
				"Env:     AUTH_TOKEN",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMCPSummary(tt.srv)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d:\n  got:  %q\n  want: %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
