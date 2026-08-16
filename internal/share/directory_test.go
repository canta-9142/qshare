package share

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDirectoryFreezesFilteredOrderedTree(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "b-dir"))
	mustMkdir(t, filepath.Join(root, "a-dir"))
	mustWrite(t, filepath.Join(root, "z.txt"), "z")
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, ".hidden"), "hidden")
	mustMkdir(t, filepath.Join(root, ".hidden-dir"))
	mustWrite(t, filepath.Join(root, ".hidden-dir", "secret"), "secret")
	if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "fifo-parent"), 0o700); err != nil {
		t.Fatal(err)
	}

	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	children := d.Root().Children()
	if got := nodeNames(children); fmt.Sprint(got) != "[a-dir b-dir fifo-parent a.txt z.txt]" {
		t.Fatalf("children = %v", got)
	}
	for _, child := range children {
		if child.ID() == "" {
			t.Fatal("empty node ID")
		}
		if d.Root().ID() == child.ID() {
			t.Fatal("duplicate node ID")
		}
	}
}

func TestOpenDirectoryRejectsSelectedSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	mustMkdir(t, target)
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDirectory(link); err == nil {
		t.Fatal("OpenDirectory() accepted symlink root")
	}
}

func TestDirectoryOpenFileRejectsReplacementAndNewFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	mustWrite(t, path, "first")
	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	node := d.Root().Children()[0]
	f, err := d.OpenFile(node)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f.Reader())
	_ = f.Close()
	if string(got) != "first" {
		t.Fatalf("content = %q", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, "replacement")
	if _, err := d.OpenFile(node); err == nil {
		t.Fatal("OpenFile() accepted replacement")
	}
	mustWrite(t, filepath.Join(root, "new.txt"), "new")
	if len(d.Root().Children()) != 1 {
		t.Fatal("tree changed after startup")
	}
}

func TestOpenDirectoryDepthBoundary(t *testing.T) {
	root := t.TempDir()
	current := root
	for i := 0; i < MaxDirectoryDepth; i++ {
		current = filepath.Join(current, "d")
		mustMkdir(t, current)
	}
	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatalf("boundary rejected: %v", err)
	}
	_ = d.Close()
	mustMkdir(t, filepath.Join(current, "too-deep"))
	if _, err := OpenDirectory(root); err == nil {
		t.Fatal("depth above limit accepted")
	}
}

func TestOpenDirectoryFileLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i <= MaxDirectoryFiles; i++ {
		mustWrite(t, filepath.Join(root, fmt.Sprintf("%04d", i)), "")
	}
	if _, err := OpenDirectory(root); err == nil {
		t.Fatal("file count above limit accepted")
	}
}

func TestOpenDirectoryRejectsDuplicateID(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "file"), "x")
	if _, err := openDirectory(root, func() (ResourceID, error) { return "same", nil }); err == nil {
		t.Fatal("duplicate ID accepted")
	}
}

func nodeNames(nodes []*Node) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name()
	}
	return names
}
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
