package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// KeyType identifies the data type of a config key.
type KeyType string

const (
	KeyTypeString KeyType = "string"
	KeyTypeBool   KeyType = "bool"
	KeyTypeEnum   KeyType = "enum"
)

// KeyDef maps a config key name to typed accessors on *Config.
type KeyDef struct {
	Name    string
	Type    KeyType
	Options []string // valid values for KeyTypeEnum
	Get     func(*Config) string
	Set     func(*Config, string) error
	Reset   func(*Config)
	Default func() string
	IsSet   func(*Config) bool
}

// keys is the ordered list of known config keys.
var keys []KeyDef

// keyMap provides O(1) lookup by key name.
var keyMap map[string]*KeyDef

func init() {
	keys = []KeyDef{
		{
			Name: "manifest_path",
			Type: KeyTypeString,
			Get: func(c *Config) string {
				if c.ManifestPath != "" {
					return c.ManifestPath
				}
				p, err := manifest.DefaultPath()
				if err != nil {
					return ""
				}
				return p
			},
			Set: func(c *Config, v string) error {
				c.ManifestPath = v
				return nil
			},
			Reset: func(c *Config) { c.ManifestPath = "" },
			Default: func() string {
				p, err := manifest.DefaultPath()
				if err != nil {
					return ""
				}
				return p
			},
			IsSet: func(c *Config) bool { return c.ManifestPath != "" },
		},
		{
			Name: "chezmoi.enabled",
			Type: KeyTypeBool,
			Get:  func(c *Config) string { return strconv.FormatBool(BoolVal(c.Chezmoi.Enabled)) },
			Set: func(c *Config, v string) error {
				b, err := strconv.ParseBool(v)
				if err != nil {
					return fmt.Errorf("invalid boolean value %q for chezmoi.enabled", v)
				}
				c.Chezmoi.Enabled = BoolPtr(b)
				return nil
			},
			Reset:   func(c *Config) { c.Chezmoi.Enabled = nil },
			Default: func() string { return "false" },
			IsSet:   func(c *Config) bool { return c.Chezmoi.Enabled != nil },
		},
		{
			Name:    "chezmoi.auto_re_add",
			Type:    KeyTypeEnum,
			Options: AutoReAddOptions,
			Get:     func(c *Config) string { return AutoReAddMode(c.Chezmoi.AutoReAdd) },
			Set: func(c *Config, v string) error {
				if !slices.Contains(AutoReAddOptions, v) {
					return fmt.Errorf("invalid value %q for chezmoi.auto_re_add (valid: %s)", v, strings.Join(AutoReAddOptions, ", "))
				}
				c.Chezmoi.AutoReAdd = StrPtr(v)
				return nil
			},
			Reset:   func(c *Config) { c.Chezmoi.AutoReAdd = nil },
			Default: func() string { return AutoReAddNever },
			IsSet:   func(c *Config) bool { return c.Chezmoi.AutoReAdd != nil },
		},
	}

	keyMap = make(map[string]*KeyDef, len(keys))
	for i := range keys {
		keyMap[keys[i].Name] = &keys[i]
	}
}

// Keys returns a copy of the ordered list of known config keys.
func Keys() []KeyDef {
	out := make([]KeyDef, len(keys))
	copy(out, keys)
	return out
}

// LookupKey returns the KeyDef for the given name.
func LookupKey(name string) (*KeyDef, error) {
	k, ok := keyMap[name]
	if !ok {
		return nil, fmt.Errorf("unknown config key %q", name)
	}
	return k, nil
}
