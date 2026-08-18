//go:build linux || darwin

package share

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenRejectsFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	if file, err := Open(path); err == nil {
		file.Close()
		t.Fatal("Open() error = nil, want error")
	}
}
