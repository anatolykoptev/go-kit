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

// stderrWarningPlusError is a multi-block stderr with a warning (multi-line
// with hints) followed by an error with location. Ported from Dadido3/go-typst
// TestErrorParsing "Typst 0.13.0 HTML warning + error".
const stderrWarningPlusError = "warning: html export is under active development and incomplete\n" +
	" = hint: its behaviour may change at any time\n" +
	" = hint: do not rely on this feature for production use cases\n" +
	" = hint: see https://github.com/typst/typst/issues/5512 for more information\n\n" +
	"error: page configuration is not allowed inside of containers\n" +
	"  ┌─ <stdin>:1:1\n" +
	"  │\n" +
	"1 │ #set page(width: 100mm, height: auto, margin: 5mm)\n" +
	"  │  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^\n\n"

// stderrMultipleErrors has two error blocks with locations. Ported from
// Dadido3/go-typst TestErrorParsing "Typst 0.13.0 multiple errors with paths".
const stderrMultipleErrors = "error: expected expression\n" +
	"  ┌─ <stdin>:11:53\n" +
	"  │\n" +
	"11 │ - Uses stdio; No temporary files need to be created.#\n" +
	"  │                                                      ^\n\n" +
	"error: expected expression\n" +
	"  ┌─ <stdin>:12:34\n" +
	"  │\n" +
	"12 │ - Test coverage of most features.#\n" +
	"  │                                   ^\n\n"

// stderrStackedHelp has an error block followed by a help: block with its own
// location. Ported from Dadido3/go-typst TestErrorParsing "Typst 0.13.0 stacked
// errors with paths". Tests that the help: prefix is parsed (not just error:).
const stderrStackedHelp = "error: expected expression\n" +
	"  ┌─ test.typ:1:4\n" +
	"  │\n" +
	"1 │ hey#\n" +
	"  │     ^\n\n" +
	"help: error occurred while importing this module\n" +
	"  ┌─ <stdin>:14:9\n" +
	"  │\n" +
	"14 │ #include \"test.typ\"\n" +
	"  │          ^^^^^^^^^^\n\n"

func TestParseStderr_WarningPlusError(t *testing.T) {
	ce := ParseStderr(stderrWarningPlusError, nil)
	if ce == nil {
		t.Fatal("ParseStderr returned nil for multi-block stderr")
	}
	if len(ce.Details) != 2 {
		t.Fatalf("expected 2 details (warning + error), got %d", len(ce.Details))
	}
	// First block: warning with multi-line message (hints).
	w := ce.Details[0]
	if !strings.HasPrefix(w.Message, "warning: html export") {
		t.Errorf("detail[0] Message: got %q", w.Message)
	}
	if !strings.Contains(w.Message, "= hint:") {
		t.Errorf("detail[0] should include hint lines in Message, got %q", w.Message)
	}
	if w.Path != "" || w.Line != 0 || w.Column != 0 {
		t.Errorf("warning has no location, got path=%q line=%d col=%d", w.Path, w.Line, w.Column)
	}
	// Second block: error with location.
	e := ce.Details[1]
	if !strings.HasPrefix(e.Message, "error: page configuration") {
		t.Errorf("detail[1] Message: got %q", e.Message)
	}
	if e.Path != "<stdin>" || e.Line != 1 || e.Column != 1 {
		t.Errorf("detail[1] location: got path=%q line=%d col=%d, want <stdin>/1/1", e.Path, e.Line, e.Column)
	}
}

func TestParseStderr_MultipleErrors(t *testing.T) {
	ce := ParseStderr(stderrMultipleErrors, nil)
	if ce == nil {
		t.Fatal("ParseStderr returned nil")
	}
	if len(ce.Details) != 2 {
		t.Fatalf("expected 2 details, got %d", len(ce.Details))
	}
	if ce.Details[0].Line != 11 || ce.Details[0].Column != 53 {
		t.Errorf("detail[0]: got line=%d col=%d, want 11/53", ce.Details[0].Line, ce.Details[0].Column)
	}
	if ce.Details[1].Line != 12 || ce.Details[1].Column != 34 {
		t.Errorf("detail[1]: got line=%d col=%d, want 12/34", ce.Details[1].Line, ce.Details[1].Column)
	}
}

func TestParseStderr_StackedHelp(t *testing.T) {
	ce := ParseStderr(stderrStackedHelp, nil)
	if ce == nil {
		t.Fatal("ParseStderr returned nil")
	}
	if len(ce.Details) != 2 {
		t.Fatalf("expected 2 details (error + help), got %d", len(ce.Details))
	}
	// First block: error.
	if ce.Details[0].Path != "test.typ" || ce.Details[0].Line != 1 || ce.Details[0].Column != 4 {
		t.Errorf("detail[0] (error): got path=%q line=%d col=%d, want test.typ/1/4",
			ce.Details[0].Path, ce.Details[0].Line, ce.Details[0].Column)
	}
	// Second block: help with its own location.
	h := ce.Details[1]
	if !strings.HasPrefix(h.Message, "help:") {
		t.Errorf("detail[1] should be a help: block, got Message %q", h.Message)
	}
	if h.Path != "<stdin>" || h.Line != 14 || h.Column != 9 {
		t.Errorf("detail[1] (help) location: got path=%q line=%d col=%d, want <stdin>/14/9",
			h.Path, h.Line, h.Column)
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

// stderrPanicWithCallStack is a real typst 0.14.2 stderr for a panic with a
// recursive call stack. The error block is followed by multiple help: blocks,
// each with its own source location. Empirically captured — typst 0.14.2 uses
// "help:" prefix for call-stack traces, NOT "while calling" (which the reviewer
// claimed from an unmerged typst PR).
const stderrPanicWithCallStack = "error: panicked with: \"boom\"\n" +
	"  ┌─ <stdin>:1:26\n" +
	"  │\n" +
	"1 │ #let f(n) = { if n == 0 { panic(\"boom\") } else { f(n - 1) } }\n" +
	"  │                           ^^^^^^^^^^^^^\n\n" +
	"help: error occurred in this call of function `f`\n" +
	"  ┌─ <stdin>:1:49\n" +
	"  │\n" +
	"1 │ #let f(n) = { if n == 0 { panic(\"boom\") } else { f(n - 1) } }\n" +
	"  │                                                  ^^^^^^^^\n\n" +
	"help: error occurred in this call of function `f`\n" +
	"  ┌─ <stdin>:2:1\n" +
	"  │\n" +
	"2 │ #f(5)\n" +
	"  │  ^^^^\n\n"

func TestParseStderr_PanicWithCallStack(t *testing.T) {
	ce := ParseStderr(stderrPanicWithCallStack, nil)
	if ce == nil {
		t.Fatal("ParseStderr returned nil for panic + call stack")
	}
	if len(ce.Details) != 3 {
		t.Fatalf("expected 3 details (1 error + 2 help), got %d", len(ce.Details))
	}
	// Error block.
	if !strings.HasPrefix(ce.Details[0].Message, "error: panicked with") {
		t.Errorf("detail[0] should be the panic error, got %q", ce.Details[0].Message)
	}
	if ce.Details[0].Line != 1 || ce.Details[0].Column != 26 {
		t.Errorf("detail[0] location: got line=%d col=%d, want 1/26", ce.Details[0].Line, ce.Details[0].Column)
	}
	// First help: block (recursive call).
	if !strings.HasPrefix(ce.Details[1].Message, "help:") {
		t.Errorf("detail[1] should be a help: block, got %q", ce.Details[1].Message)
	}
	if ce.Details[1].Line != 1 || ce.Details[1].Column != 49 {
		t.Errorf("detail[1] location: got line=%d col=%d, want 1/49", ce.Details[1].Line, ce.Details[1].Column)
	}
	// Second help: block (top-level call).
	if ce.Details[2].Line != 2 || ce.Details[2].Column != 1 {
		t.Errorf("detail[2] location: got line=%d col=%d, want 2/1", ce.Details[2].Line, ce.Details[2].Column)
	}
}
