// Package ui provides shared terminal interaction utilities.
package ui

import (
	"fmt"
	"io"
	"strings"
)

// Confirm prints a prompt to w and reads y/n from r.
// Returns true for "y" or "yes" (case-insensitive).
// If force is true, skips the prompt and returns true.
//
// Reads byte-by-byte so that multiple Confirm calls on the same reader
// (e.g., piped input) consume only their own line without buffering ahead.
func Confirm(r io.Reader, w io.Writer, prompt string, force bool) (bool, error) {
	if force {
		return true, nil
	}

	_, _ = fmt.Fprintf(w, "%s [y/N] ", prompt)

	line, err := readLine(r)
	if err != nil {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}

	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

// readLine reads one line from r byte-by-byte, returning the line content
// without the trailing newline. Returns "" on EOF with no error.
func readLine(r io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 && buf[0] == '\n' {
			break
		}
		if n > 0 {
			line = append(line, buf[0])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return string(line), nil
}
