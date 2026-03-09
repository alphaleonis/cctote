// Package cliutil provides a generic command runner interface for shelling out
// to external CLIs, along with an exec-based implementation.
package cliutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes a command and returns its stdout. On non-zero exit, the
// returned error should include stderr content for diagnostics.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// RunError is returned by ExecRunner when the command exits with a non-zero
// status or is otherwise interrupted (e.g. context cancellation). It captures
// stderr separately so callers can extract the human-readable message without
// parsing the full error chain.
type RunError struct {
	Command string
	// Args holds the arguments passed to Command, available for structured
	// logging or diagnostics. Not included in Error() output.
	Args     []string
	ExitCode int    // -1 when exit code is unavailable (e.g. signal kill)
	Stderr   string // trimmed stderr output, may be empty
	Err      error  // underlying error (*exec.ExitError or context error)
}

func (e *RunError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("running %s: %v: %s", e.Command, e.Err, e.Stderr)
	}
	return fmt.Sprintf("running %s: %v", e.Command, e.Err)
}

func (e *RunError) Unwrap() error { return e.Err }

// ExecRunner implements Runner by executing an external command via os/exec.
type ExecRunner struct {
	Command string
}

// Run executes the command with the given args, returning stdout bytes. On
// non-zero exit, the error is a *RunError that captures stderr separately.
func (r *ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return nil, &RunError{
			Command:  r.Command,
			Args:     args,
			ExitCode: exitCode,
			Stderr:   string(bytes.TrimSpace(stderr.Bytes())),
			Err:      err,
		}
	}
	return stdout.Bytes(), nil
}

// UserMessage extracts a human-readable message from an error for display in
// the TUI. It walks the error chain looking for *RunError and returns its
// Stderr (the CLI's user-facing output), falling back to the full error string.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}

	// errors.Join: recurse on each sub-error, collecting UserMessages.
	// Must check before errors.As because As traverses joined errors and
	// would only return the first RunError, losing the rest.
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var msgs []string
		for _, sub := range joined.Unwrap() {
			if sub != nil {
				msgs = append(msgs, UserMessage(sub))
			}
		}
		return strings.Join(msgs, "\n")
	}

	// RunError: prefer stderr.
	var runErr *RunError
	if errors.As(err, &runErr) {
		if runErr.Stderr != "" {
			return runErr.Stderr
		}
		return runErr.Error()
	}

	return err.Error()
}

// LookPath checks whether command is available on PATH.
func LookPath(command string) error {
	_, err := exec.LookPath(command)
	if err != nil {
		return fmt.Errorf("finding %s on PATH: %w", command, err)
	}
	return nil
}
