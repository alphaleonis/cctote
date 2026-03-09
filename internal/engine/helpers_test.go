package engine

import (
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestWarnSecretEnvVars_MatchesPatterns(t *testing.T) {
	servers := map[string]manifest.MCPServer{
		"safe": {Command: "echo", Env: map[string]string{"PORT": "3000"}},
		"risky": {Command: "node", Env: map[string]string{
			"OPENAI_API_KEY": "sk-xxx",
			"PORT":           "8080",
		}},
	}
	hooks := &mockHooks{}
	WarnSecretEnvVars(servers, []string{"safe", "risky"}, hooks)

	if len(hooks.warnCalls) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(hooks.warnCalls))
	}
	if !strings.Contains(hooks.warnCalls[0], "OPENAI_API_KEY") {
		t.Errorf("warning should mention OPENAI_API_KEY: %s", hooks.warnCalls[0])
	}
	if strings.Contains(hooks.warnCalls[0], "PORT") {
		t.Errorf("warning should not mention PORT: %s", hooks.warnCalls[0])
	}
}

func TestWarnSecretEnvVars_NoSecrets(t *testing.T) {
	servers := map[string]manifest.MCPServer{
		"safe": {Command: "echo", Env: map[string]string{"PORT": "3000", "DEBUG": "true"}},
	}
	hooks := &mockHooks{}
	WarnSecretEnvVars(servers, []string{"safe"}, hooks)

	if len(hooks.warnCalls) != 0 {
		t.Fatalf("expected no warnings, got %d: %v", len(hooks.warnCalls), hooks.warnCalls)
	}
}

func TestWarnSecretEnvVars_NoEnv(t *testing.T) {
	servers := map[string]manifest.MCPServer{
		"plain": {Command: "echo"},
	}
	hooks := &mockHooks{}
	WarnSecretEnvVars(servers, []string{"plain"}, hooks)

	if len(hooks.warnCalls) != 0 {
		t.Fatalf("expected no warnings, got %d", len(hooks.warnCalls))
	}
}

func TestWarnSecretEnvVars_MultipleServers(t *testing.T) {
	servers := map[string]manifest.MCPServer{
		"a": {Command: "a", Env: map[string]string{"SECRET_KEY": "s1"}},
		"b": {Command: "b", Env: map[string]string{"AUTH_TOKEN": "t1"}},
	}
	hooks := &mockHooks{}
	WarnSecretEnvVars(servers, []string{"a", "b"}, hooks)

	if len(hooks.warnCalls) != 1 {
		t.Fatalf("expected 1 consolidated warning, got %d", len(hooks.warnCalls))
	}
	warn := hooks.warnCalls[0]
	if !strings.Contains(warn, "SECRET_KEY") || !strings.Contains(warn, "AUTH_TOKEN") {
		t.Errorf("warning should mention both keys: %s", warn)
	}
}

func TestWarnSecretEnvVars_CaseInsensitive(t *testing.T) {
	servers := map[string]manifest.MCPServer{
		"srv": {Command: "x", Env: map[string]string{"my_api_key": "val"}},
	}
	hooks := &mockHooks{}
	WarnSecretEnvVars(servers, []string{"srv"}, hooks)

	if len(hooks.warnCalls) != 1 {
		t.Fatalf("expected 1 warning for lowercase api_key, got %d", len(hooks.warnCalls))
	}
}

func TestLooksLikeSecret(t *testing.T) {
	positives := []string{
		"API_KEY", "OPENAI_API_KEY", "api_key", "My_ApiKey",
		"SECRET", "CLIENT_SECRET", "secret_value",
		"TOKEN", "AUTH_TOKEN", "access_token",
		"PASSWORD", "DB_PASSWORD", "passwd",
		"CREDENTIAL", "GCP_CREDENTIAL",
		"PRIVATE_KEY", "SSH_PRIVATE_KEY",
		"ACCESS_KEY", "AWS_ACCESS_KEY_ID",
	}
	for _, k := range positives {
		if !looksLikeSecret(k) {
			t.Errorf("expected %q to look like a secret", k)
		}
	}

	negatives := []string{
		"PORT", "HOST", "DEBUG", "LOG_LEVEL", "PATH", "HOME",
		"DATABASE_URL", "NODE_ENV", "EDITOR",
	}
	for _, k := range negatives {
		if looksLikeSecret(k) {
			t.Errorf("expected %q NOT to look like a secret", k)
		}
	}
}
