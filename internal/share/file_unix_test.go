//go:build linux || darwin

package share

import (
	"path/filepath"
	"testing"
	"time"

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

func TestOpenFileNoFollowDoesNotBlockOnFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		file, err := openFileNoFollow(path)
		if file != nil {
			_ = file.Close()
		}
		result <- err
	}()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("openFileNoFollow() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		// Release a potentially blocked reader so a failing test does not
		// leave a goroutine stuck in open.
		fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if err == nil {
			_ = unix.Close(fd)
		}
		select {
		case <-result:
		case <-time.After(time.Second):
		}
		t.Fatal("openFileNoFollow() blocked while opening a FIFO")
	}
}
