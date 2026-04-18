package breaker

import (
	"errors"
	"io"
	"testing"
	"time"
)

func TestExecute_RunsFunctionWhenClosed(t *testing.T) {
	b := New(Options{FailThreshold: 3, OpenDuration: time.Second})
	got, err := Execute(b, func() (int, error) { return 42, nil })
	if err != nil || got != 42 {
		t.Fatalf("got (%d, %v), want (42, nil)", got, err)
	}
}

func TestExecute_ReturnsErrOpenWhenTripped(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: time.Second})
	_, _ = Execute(b, func() (int, error) { return 0, io.EOF })
	_, err := Execute(b, func() (int, error) { return 42, nil })
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
}

func TestExecute_RecordsFailureOnError(t *testing.T) {
	b := New(Options{FailThreshold: 2, OpenDuration: time.Second})
	for range 2 {
		_, _ = Execute(b, func() (int, error) { return 0, io.EOF })
	}
	if b.State() != StateOpen {
		t.Fatalf("state = %s, want open after 2 errors", b.State())
	}
}
