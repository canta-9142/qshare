package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canta-9142/qshare/internal/share"
)

func TestRunPathSelectionExitCodes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	tooDeep := t.TempDir()
	child := tooDeep
	for i := 0; i <= share.MaxDirectoryDepth; i++ {
		child = filepath.Join(child, "child")
	}
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	tooMany := make([]string, share.MaxFiles+1)
	for i := range tooMany {
		tooMany[i] = missing
	}
	for _, tt := range []struct {
		name       string
		paths      []string
		code       int
		diagnostic string
	}{
		{"directory and file", []string{dir, file}, 2, "a directory cannot be combined"},
		{"file and directory", []string{file, dir}, 2, "a directory cannot be combined"},
		{"directories", []string{dir, dir}, 2, "a directory cannot be combined"},
		{"directory and missing", []string{dir, missing}, 2, "a directory cannot be combined"},
		{"missing and directory", []string{missing, dir}, 2, "a directory cannot be combined"},
		{"too many paths", tooMany, 2, "too many files"},
		{"missing", []string{missing}, 1, "lstat shared file"},
		{"partial files", []string{file, missing}, 1, "lstat shared file"},
		{"directory symlink", []string{link}, 1, "not a regular file"},
		{"directory limit", []string{tooDeep}, 1, "directory depth exceeds"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(tt.paths, &stdout, &stderr); code != tt.code {
				t.Fatalf("Run() = %d, want %d; stderr=%q", code, tt.code, stderr.String())
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), tt.diagnostic) {
				t.Fatalf("stdout=%q stderr=%q, want diagnostic %q on stderr only", stdout.String(), stderr.String(), tt.diagnostic)
			}
		})
	}
}

func TestRunWithInputDoesNotStartQuitListenerBeforeSessionReady(t *testing.T) {
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
	if starts != 0 || listener.closeCalls != 0 {
		t.Fatalf("listener starts=%d closes=%d, want starts=0 closes=0", starts, listener.closeCalls)
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
