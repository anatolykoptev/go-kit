package stt

import (
	"errors"
	"strings"
	"testing"
)

// failingWriter succeeds for the first limit bytes, then returns err on every
// subsequent Write. It is used to inject a failure into the multipart.Writer
// so that WriteField errors are observable.
type failingWriter struct {
	written int
	limit   int
	err     error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.err == nil {
		w.err = errors.New("write failure injected")
	}
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, w.err
	}
	if len(p) <= remaining {
		w.written += len(p)
		return len(p), nil
	}
	w.written = w.limit
	return remaining, w.err
}

// TestBuildMultipartWriteFieldFailure verifies that a failing io.Writer causes
// buildMultipart to return a wrapped "write field" error rather than silently
// producing a malformed body. The writer allows the file part to complete but
// fails during WriteField.
//
// This test goes RED when the WriteField error-propagation guard is removed:
// without propagation the WriteField error is ignored and the only error
// returned would be from Close() (wrapped as "close writer"), which does not
// contain "write field".
func TestBuildMultipartWriteFieldFailure(t *testing.T) {
	t.Parallel()

	c := New("http://127.0.0.1:1",
		WithPunctuate(true),
		WithSmartFormat(true),
		WithDiarize(true),
		WithDiarizeSpeakers(2), //nolint:mnd
		WithKeywords([]string{"foo", "bar"}),
		WithCustomSpelling(map[string]string{"foo": "bar"}),
	)

	// limit is large enough for the file part (header + audio) to succeed but
	// small enough that the subsequent WriteField calls hit the failure.
	fw := &failingWriter{limit: 250} //nolint:mnd
	audio := strings.NewReader("audio")

	_, err := c.buildMultipart(fw, audio, "audio.wav", "")
	if err == nil {
		t.Fatal("expected buildMultipart to return an error when WriteField fails, got nil")
	}
	if !strings.Contains(err.Error(), "write field") {
		t.Fatalf("expected error to be a WriteField error (contains \"write field\"), got: %v", err)
	}
	if !errors.Is(err, fw.err) {
		t.Errorf("expected error to wrap injected write failure, got: %v", err)
	}
}

// TestBuildMultipartWriteFieldFailureOptionalFields verifies that a failure
// during an optional field (punctuate, smart_format, diarize, etc.) is also
// propagated as a "write field" error.
func TestBuildMultipartWriteFieldFailureOptionalFields(t *testing.T) {
	t.Parallel()

	c := New("http://127.0.0.1:1",
		WithPunctuate(true),
		WithSmartFormat(true),
		WithDiarize(true),
		WithDiarizeSpeakers(2), //nolint:mnd
		WithKeywords([]string{"foo"}),
		WithCustomSpelling(map[string]string{"foo": "bar"}),
	)

	// limit allows the file part + mandatory fields (model, language,
	// response_format) to complete, but fails during optional fields.
	fw := &failingWriter{limit: 600} //nolint:mnd
	audio := strings.NewReader("audio")

	_, err := c.buildMultipart(fw, audio, "audio.wav", "")
	if err == nil {
		t.Fatal("expected buildMultipart to return an error when an optional WriteField fails, got nil")
	}
	if !strings.Contains(err.Error(), "write field") {
		t.Fatalf("expected error to be a WriteField error (contains \"write field\"), got: %v", err)
	}
	if !errors.Is(err, fw.err) {
		t.Errorf("expected error to wrap injected write failure, got: %v", err)
	}
}
