package cliutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestExecRunnerSuccess(t *testing.T) {
	r := &ExecRunner{Command: "echo"}
	out, err := r.Run(context.Background(), "hello", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestExecRunnerFailure(t *testing.T) {
	r := &ExecRunner{Command: "false"}
	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from false command")
	}
	if !strings.Contains(err.Error(), "false") {
		t.Errorf("error %q should mention the command name", err)
	}

	// Verify it's a *RunError.
	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatal("expected *RunError")
	}
	if runErr.Command != "false" {
		t.Errorf("Command = %q, want %q", runErr.Command, "false")
	}
	if runErr.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", runErr.ExitCode)
	}
}

func TestExecRunnerStderrInError(t *testing.T) {
	r := &ExecRunner{Command: "sh"}
	_, err := r.Run(context.Background(), "-c", "echo diagnostic >&2; exit 1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "diagnostic") {
		t.Errorf("error %q should contain stderr content", err)
	}

	// Verify RunError fields.
	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatal("expected *RunError")
	}
	if runErr.Stderr != "diagnostic" {
		t.Errorf("Stderr = %q, want %q", runErr.Stderr, "diagnostic")
	}
	if runErr.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", runErr.ExitCode)
	}
}

func TestExecRunnerNotFound(t *testing.T) {
	r := &ExecRunner{Command: "cctote-nonexistent-binary-xyz"}
	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestExecRunnerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &ExecRunner{Command: "sleep"}
	_, err := r.Run(ctx, "10")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRunErrorUnwrapThroughWrapping(t *testing.T) {
	runErr := &RunError{
		Command:  "claude",
		ExitCode: 1,
		Stderr:   "Plugin not found",
		Err:      fmt.Errorf("exit status 1"),
	}
	wrapped := fmt.Errorf("installing plugin %q: %w", "test-plugin", runErr)

	var extracted *RunError
	if !errors.As(wrapped, &extracted) {
		t.Fatal("errors.As should find *RunError through wrapping")
	}
	if extracted.Stderr != "Plugin not found" {
		t.Errorf("Stderr = %q, want %q", extracted.Stderr, "Plugin not found")
	}
}

func TestUserMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "RunError with stderr",
			err: &RunError{
				Command:  "claude",
				ExitCode: 1,
				Stderr:   "Plugin \"foo\" not found in marketplace \"bar\"",
				Err:      fmt.Errorf("exit status 1"),
			},
			want: "Plugin \"foo\" not found in marketplace \"bar\"",
		},
		{
			name: "RunError empty stderr falls back",
			err: &RunError{
				Command:  "claude",
				ExitCode: 1,
				Stderr:   "",
				Err:      fmt.Errorf("exit status 1"),
			},
			want: "running claude: exit status 1",
		},
		{
			name: "RunError through wrapping",
			err: fmt.Errorf("installing plugin %q: %w", "test", &RunError{
				Command:  "claude",
				ExitCode: 1,
				Stderr:   "Not found",
				Err:      fmt.Errorf("exit status 1"),
			}),
			want: "Not found",
		},
		{
			name: "errors.Join with RunErrors",
			err: errors.Join(
				fmt.Errorf("installing plugin %q: %w", "a", &RunError{
					Command: "claude", ExitCode: 1, Stderr: "Error A",
					Err: fmt.Errorf("exit status 1"),
				}),
				fmt.Errorf("installing plugin %q: %w", "b", &RunError{
					Command: "claude", ExitCode: 1, Stderr: "Error B",
					Err: fmt.Errorf("exit status 1"),
				}),
			),
			want: "Error A\nError B",
		},
		{
			name: "plain error fallback",
			err:  fmt.Errorf("some other error"),
			want: "some other error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UserMessage(tt.err)
			if got != tt.want {
				t.Errorf("UserMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLookPathExists(t *testing.T) {
	if err := LookPath("echo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLookPathNotFound(t *testing.T) {
	err := LookPath("cctote-nonexistent-binary-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "finding") || !strings.Contains(err.Error(), "PATH") {
		t.Errorf("error %q should mention 'finding' and 'PATH'", err)
	}
}
