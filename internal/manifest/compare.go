package manifest

// MCPServersEqual compares two MCPServer values, treating nil and empty
// slices/maps as equivalent. This is necessary because JSON round-tripping
// through omitempty can convert empty values to nil.
func MCPServersEqual(a, b MCPServer) bool {
	return a.Type == b.Type &&
		a.Command == b.Command &&
		stringSlicesEqual(a.Args, b.Args) &&
		a.CWD == b.CWD &&
		stringMapsEqual(a.Env, b.Env) &&
		a.URL == b.URL &&
		stringMapsEqual(a.Headers, b.Headers) &&
		oauthEqual(a.OAuth, b.OAuth)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			return false
		}
	}
	return true
}

// MarketplacesEqual compares two Marketplace values. All fields are scalar
// strings, so a simple field comparison is sufficient.
func MarketplacesEqual(a, b Marketplace) bool {
	return a.Source == b.Source &&
		a.Repo == b.Repo &&
		a.URL == b.URL &&
		a.Path == b.Path
}

// PluginsEqual compares two Plugin values. All fields are scalar, so a
// simple field comparison is sufficient.
func PluginsEqual(a, b Plugin) bool {
	return a.ID == b.ID &&
		a.Scope == b.Scope &&
		a.Enabled == b.Enabled
}

func oauthEqual(a, b *MCPOAuth) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ClientID == b.ClientID &&
		a.CallbackPort == b.CallbackPort
}
