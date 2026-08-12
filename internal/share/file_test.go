package share

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.txt")
	content := []byte("qshare test content")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	file, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	if got, want := file.Name(), filepath.Base(path); got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if got, want := file.Size(), int64(len(content)); got != want {
		t.Errorf("Size() = %d, want %d", got, want)
	}
	if file.ModTime().IsZero() {
		t.Error("ModTime() is zero")
	}

	got, err := io.ReadAll(file.Reader())
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Reader() content = %q, want %q", got, content)
	}
}

func TestFileReadersHaveIndependentOffsets(t *testing.T) {
	path := writeTestFile(t, "abcdef")
	file, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	first := file.Reader()
	second := file.Reader()
	buffer := make([]byte, 2)
	if _, err := io.ReadFull(first, buffer); err != nil {
		t.Fatalf("read first reader: %v", err)
	}
	if _, err := io.ReadFull(second, buffer); err != nil {
		t.Fatalf("read second reader: %v", err)
	}
	if got := string(buffer); got != "ab" {
		t.Errorf("second reader starts with %q, want %q", got, "ab")
	}
}

func TestOpenPinsSelectedFileAfterPathReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.txt")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := os.Rename(path, filepath.Join(dir, "old.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(file.Reader())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("Reader() = %q, want original file", got)
	}
}

func TestOpenRejectsUnsupportedPaths(t *testing.T) {
	dir := t.TempDir()
	regularPath := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(regularPath, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	symlinkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(regularPath, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	fifoPath := filepath.Join(dir, "pipe")
	if err := unix.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "missing", path: filepath.Join(dir, "missing.txt")},
		{name: "directory", path: dir},
		{name: "final symlink", path: symlinkPath},
		{name: "FIFO", path: fifoPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if file, err := Open(tt.path); err == nil {
				file.Close()
				t.Fatal("Open() error = nil, want error")
			}
		})
	}
}

func TestOpenAllowsSymlinkInAncestorDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	path := filepath.Join(realDir, "example.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	linkedDir := filepath.Join(root, "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	file, err := Open(filepath.Join(linkedDir, "example.txt"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func writeTestFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
