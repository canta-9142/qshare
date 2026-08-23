package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunWithInputClosesQuitListenerAfterRuntimeFailure(t *testing.T) {
	listener := &fakeTerminalQuitListener{quit: make(chan struct{})}
	starts := 0
	start := func() (terminalQuitListener, error) {
		starts++
		return listener, nil
	}
	var stderr bytes.Buffer

	code := runWithInputAndQuitListener(
		[]string{"missing-file"},
		"devel",
		nil,
		true,
		&bytes.Buffer{},
		&stderr,
		start,
	)

	if code != 1 {
		t.Fatalf("runWithInputAndQuitListener() = %d, want 1", code)
	}
	if starts != 1 || listener.closeCalls != 1 {
		t.Fatalf("listener starts=%d closes=%d, want starts=1 closes=1", starts, listener.closeCalls)
	}
}

func TestRunWithInputReportsQuitListenerStartupFailure(t *testing.T) {
	want := errors.New("terminal failed")
	var stderr bytes.Buffer

	code := runWithInputAndQuitListener(
		[]string{"missing-file"},
		"devel",
		nil,
		true,
		&bytes.Buffer{},
		&stderr,
		func() (terminalQuitListener, error) { return nil, want },
	)

	if code != 1 {
		t.Fatalf("runWithInputAndQuitListener() = %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "configure quit key: terminal failed") {
		t.Fatalf("stderr = %q, want quit-listener diagnostic", got)
	}
}

func TestRunWithInputDoesNotStartQuitListenerForHelp(t *testing.T) {
	starts := 0
	code := runWithInputAndQuitListener(
		[]string{"--help"},
		"devel",
		nil,
		true,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func() (terminalQuitListener, error) {
			starts++
			return nil, errors.New("unexpected listener start")
		},
	)

	if code != 0 {
		t.Fatalf("runWithInputAndQuitListener() = %d, want 0", code)
	}
	if starts != 0 {
		t.Fatalf("listener starts = %d, want 0", starts)
	}
}

func TestRunWithInputDoesNotStartQuitListenerForVersion(t *testing.T) {
	starts := 0
	var stdout bytes.Buffer
	code := runWithInputAndQuitListener(
		[]string{"--version"},
		"v1.2.3",
		nil,
		true,
		&stdout,
		&bytes.Buffer{},
		func() (terminalQuitListener, error) {
			starts++
			return nil, errors.New("unexpected listener start")
		},
	)

	if code != 0 {
		t.Fatalf("runWithInputAndQuitListener() = %d, want 0", code)
	}
	if starts != 0 {
		t.Fatalf("listener starts = %d, want 0", starts)
	}
	if got := stdout.String(); got != "qshare v1.2.3\n" {
		t.Fatalf("stdout = %q, want %q", got, "qshare v1.2.3\n")
	}
}

type fakeTerminalQuitListener struct {
	quit       chan struct{}
	closeCalls int
	closeErr   error
}

func (l *fakeTerminalQuitListener) Quit() <-chan struct{} {
	return l.quit
}

func (l *fakeTerminalQuitListener) Close() error {
	l.closeCalls++
	return l.closeErr
}
