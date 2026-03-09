package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

func TestConfirmForceSkipsPrompt(t *testing.T) {
	var out bytes.Buffer
	got, err := Confirm(strings.NewReader(""), &out, "Delete?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true when force=true")
	}
	if out.Len() != 0 {
		t.Errorf("expected no output when force=true, got %q", out.String())
	}
}

func TestConfirmYes(t *testing.T) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  y  \n"} {
		t.Run(input, func(t *testing.T) {
			var out bytes.Buffer
			got, err := Confirm(strings.NewReader(input), &out, "Continue?", false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got {
				t.Error("expected true")
			}
		})
	}
}

func TestConfirmNo(t *testing.T) {
	for _, input := range []string{"n\n", "N\n", "no\n", "anything\n", "\n"} {
		t.Run(input, func(t *testing.T) {
			var out bytes.Buffer
			got, err := Confirm(strings.NewReader(input), &out, "Continue?", false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got {
				t.Error("expected false")
			}
		})
	}
}

func TestConfirmEOF(t *testing.T) {
	var out bytes.Buffer
	got, err := Confirm(strings.NewReader(""), &out, "Continue?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false on EOF")
	}
}

func TestConfirmPromptText(t *testing.T) {
	var out bytes.Buffer
	got, err := Confirm(strings.NewReader("n\n"), &out, "Delete everything?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for 'n' input")
	}
	want := "Delete everything? [y/N] "
	if out.String() != want {
		t.Errorf("prompt = %q, want %q", out.String(), want)
	}
}

func TestConfirmMultipleCallsSameReader(t *testing.T) {
	// Simulate piped input with two "yes" responses.
	// Both Confirm calls must share the same reader without losing data.
	r := strings.NewReader("y\ny\n")
	var out bytes.Buffer

	got1, err := Confirm(r, &out, "First?", false)
	if err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	if !got1 {
		t.Error("first call: expected true for 'y'")
	}

	got2, err := Confirm(r, &out, "Second?", false)
	if err != nil {
		t.Fatalf("second Confirm: %v", err)
	}
	if !got2 {
		t.Error("second call: expected true for 'y' (got false; likely lost to buffering)")
	}
}

func TestConfirmReaderError(t *testing.T) {
	var out bytes.Buffer
	got, err := Confirm(iotest.ErrReader(errors.New("broken pipe")), &out, "Continue?", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reading confirmation") {
		t.Errorf("error = %q, want it to contain 'reading confirmation'", err)
	}
	if got {
		t.Error("expected false on reader error")
	}
}
