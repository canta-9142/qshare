//go:build linux || darwin

package share

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestOpenDirectoryExcludesFIFO(t *testing.T) {
	root := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	directory, err := OpenDirectory(root)
	if err != nil {
		t.Fatalf("OpenDirectory() error = %v", err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	if children := directory.Root().Children(); len(children) != 0 {
		t.Fatalf("root children = %v, want none", nodeNames(children))
	}
}

func TestDirectoryOpenFileRejectsFIFOReplacementWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file")
	mustWrite(t, path, "content")
	directory, err := OpenDirectory(root)
	if err != nil {
		t.Fatalf("OpenDirectory() error = %v", err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	node := directory.Root().Children()[0]

	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		file, err := directory.OpenFile(node)
		if file != nil {
			_ = file.Close()
		}
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("OpenFile() error = nil, want replacement error")
		}
	case <-time.After(2 * time.Second):
		// Release a potentially blocked FIFO reader so a failing test does not
		// leave a goroutine stuck in openat.
		fd, openErr := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if openErr == nil {
			_ = unix.Close(fd)
		}
		select {
		case <-result:
		case <-time.After(time.Second):
		}
		t.Fatal("OpenFile() blocked while opening a FIFO replacement")
	}
}
