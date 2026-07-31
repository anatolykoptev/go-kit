package typst

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// stderrFileNotFound is a real typst 0.14.2 stderr for a missing-image error,
// captured with --diagnostic-format human. Used to test ParseStderr without
// invoking the binary (deterministic, no typst dependency).
const stderrFileNotFound = `error: file not found (searched at /tmp/doc/nonexistent.png)
  ┌─ <stdin>:2:7
  │
2 │ #image("nonexistent.png")
  │        ^^^^^^^^^^^^^^^^^

`

// stderrSyntaxError is a real typst 0.14.2 stderr for an incomplete expression.
const stderrSyntaxError = `error: expected expression
  ┌─ <stdin>:2:8
  │
2 │ #let x = 
  │         ^

`

// stderrNoLocation is a synthetic block without the ┌─ location line — tests
// the regex's optional-path branch. Typst panics and some internal errors
// omit the source location.
const stderrNoLocation = `error: typst crashed unexpectedly

`

func TestParseStderr_FileNotFound(t *testing.T) {
	ce := ParseStderr(stderrFileNotFound, errors.New("exit status 1"))
	if ce == nil {
		t.Fatal("ParseStderr returned nil for a valid diagnostic block")
	}
	if len(ce.Details) != 1 {
		t.Fatalf("expected 1 detail, got %d: %+v", len(ce.Details), ce.Details)
	}
	d := ce.Details[0]
	if d.Message != "error: file not found (searched at /tmp/doc/nonexistent.png)" {
		t.Errorf("Message: got %q", d.Message)
	}
	if d.Path != "<stdin>" {
		t.Errorf("Path: got %q, want <stdin>", d.Path)
	}
	if d.Line != 2 {
		t.Errorf("Line: got %d, want 2", d.Line)
	}
	if d.Column != 7 {
		t.Errorf("Column: got %d, want 7", d.Column)
	}
	if ce.Error() != stderrFileNotFound {
		t.Errorf("Error() should return Raw verbatim")
	}
	if ce.Unwrap() == nil || ce.Unwrap().Error() != "exit status 1" {
		t.Errorf("Unwrap should return the inner error")
	}
}

func TestParseStderr_SyntaxError(t *testing.T) {
	ce := ParseStderr(stderrSyntaxError, errors.New("exit status 1"))
	if ce == nil {
		t.Fatal("ParseStderr returned nil")
	}
	d := ce.Details[0]
	if d.Line != 2 || d.Column != 8 {
		t.Errorf("Line/Column: got %d/%d, want 2/8", d.Line, d.Column)
	}
	if !strings.Contains(d.Message, "expected expression") {
		t.Errorf("Message: got %q", d.Message)
	}
}

func TestParseStderr_NoLocation(t *testing.T) {
	ce := ParseStderr(stderrNoLocation, errors.New("exit status 1"))
	if ce == nil {
		t.Fatal("ParseStderr returned nil for a block without location")
	}
	if len(ce.Details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(ce.Details))
	}
	d := ce.Details[0]
	if d.Path != "" || d.Line != 0 || d.Column != 0 {
		t.Errorf("location fields should be zero, got path=%q line=%d col=%d", d.Path, d.Line, d.Column)
	}
	if !strings.Contains(d.Message, "crashed") {
		t.Errorf("Message: got %q", d.Message)
	}
}

func TestParseStderr_Empty(t *testing.T) {
	if ce := ParseStderr("", nil); ce != nil {
		t.Errorf("ParseStderr(empty) should return nil, got %+v", ce)
	}
	if ce := ParseStderr("not a diagnostic at all\njust noise", nil); ce != nil {
		t.Errorf("ParseStderr(non-diagnostic) should return nil, got %+v", ce)
	}
}

// TestCompileTypst_StructuredError is an integration test (requires typst on
// PATH) that compiles invalid source and asserts the returned error is a
// *CompileError with parsed Line/Column, accessible via errors.As.
func TestCompileTypst_StructuredError(t *testing.T) {
	skipIfNoTypstPandoc(t)
	// Missing image — typst emits a "file not found" diagnostic with location.
	src := "#image(\"nonexistent-typst-test-image.png\")\n"
	_, err := compileTypst(context.Background(), src, typstOutput{Format: typstFormatPDF})
	if err == nil {
		t.Fatal("expected error for missing image, got nil")
	}

	var ce *CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CompileError, got %T: %v", err, err)
	}
	if len(ce.Details) == 0 {
		t.Fatal("CompileError.Details is empty")
	}
	d := ce.Details[0]
	if d.Line != 1 {
		t.Errorf("Line: got %d, want 1", d.Line)
	}
	if !strings.Contains(d.Message, "file not found") {
		t.Errorf("Message should mention 'file not found', got %q", d.Message)
	}
}

// TestCompileTypst_ErrBinaryNotFound verifies that a missing typst binary
// produces an error wrapping ErrBinaryNotFound (errors.Is, not substring).
func TestCompileTypst_ErrBinaryNotFound(t *testing.T) {
	// Force resolveEnvOrPath to find nothing by clearing env vars and
	// temporarily neutering PATH. We can't unset PATH for the whole test
	// binary, so we call the lower-level resolver directly.
	t.Setenv(resolveBinaryEnvTypst, "")
	t.Setenv(legacyEnvTypst, "")
	// PATH still has typst on the krolik box; skip if typst is present.
	if _, err := exec.LookPath("typst"); err == nil {
		t.Skip("typst is on PATH — cannot test missing-binary path on this host")
	}
	_, err := compileTypst(context.Background(), "= x", typstOutput{Format: typstFormatPDF})
	if err == nil {
		t.Fatal("expected error for missing typst binary")
	}
	if !errors.Is(err, ErrBinaryNotFound) {
		t.Errorf("expected errors.Is(err, ErrBinaryNotFound), got: %v", err)
	}
}
