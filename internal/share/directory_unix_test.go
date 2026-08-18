//go:build linux || darwin

package share

import (
	"path/filepath"
	"testing"

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
