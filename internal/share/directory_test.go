package share

import (
	"errors"
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
	if err := os.Mkdir(filepath.Join(root, "empty-dir"), 0o700); err != nil {
		t.Fatal(err)
	}

	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	children := d.Root().Children()
	if got := nodeNames(children); fmt.Sprint(got) != "[a-dir b-dir empty-dir a.txt z.txt]" {
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
	for _, suffix := range []string{"", "/", "///"} {
		t.Run("suffix="+suffix, func(t *testing.T) {
			directory, err := OpenDirectory(link + suffix)
			if directory != nil {
				_ = directory.Close()
			}
			if err == nil {
				t.Fatal("OpenDirectory() accepted symlink root")
			}
		})
	}
}

func TestOpenDirectoryAllowsTrailingSeparators(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "file"), "content")
	for _, suffix := range []string{"/", "///"} {
		d, err := OpenDirectory(root + suffix)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = d.Close() })
		file, err := d.OpenFile(d.Root().Children()[0])
		if err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
	}
	if _, err := OpenDirectory(""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenDirectory(empty path) error = %v, want nonexistent path", err)
	}
}

func TestDirectoryPinsEveryNodeUntilClose(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "empty")
	mustMkdir(t, path)
	mustWrite(t, filepath.Join(root, "file"), "content")
	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	handles := []*os.File{d.root}
	for _, node := range d.byID {
		if node == d.Root() {
			continue
		}
		if node.pinned == nil {
			t.Fatalf("node %q has no retained handle", node.Name())
		}
		handles = append(handles, node.pinned)
	}
	directory := d.Root().Children()[0]
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, path)
	info, err := directory.pinned.Stat()
	if err != nil || !os.SameFile(info, directory.identity) {
		t.Fatalf("removed directory identity was not retained: %v", err)
	}
	if err := d.VerifyDirectory(directory); err == nil {
		t.Fatal("VerifyDirectory() accepted replacement directory")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	for _, handle := range handles {
		if _, err := handle.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("retained handle was not closed: %v", err)
		}
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

func TestDirectoryRejectsRenamedRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	mustMkdir(t, root)
	mustWrite(t, filepath.Join(root, "file"), "x")
	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	node := d.Root().Children()[0]
	if err := os.Rename(root, filepath.Join(parent, "moved")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.OpenFile(node); err == nil {
		t.Fatal("OpenFile() accepted renamed root")
	}
}

func TestDirectoryOpenFileRejectsReplacedAncestorWithHardLink(t *testing.T) {
	for _, replaced := range []string{"parent", filepath.Join("parent", "child")} {
		t.Run(replaced, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "parent")
			child := filepath.Join(parent, "child")
			mustMkdir(t, parent)
			mustMkdir(t, child)
			path := filepath.Join(child, "file")
			mustWrite(t, path, "content")
			d, err := OpenDirectory(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = d.Close() })
			node := d.Root().Children()[0].Children()[0].Children()[0]

			moved := filepath.Join(root, "moved")
			if err := os.Rename(filepath.Join(root, replaced), moved); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(child, 0o700); err != nil {
				t.Fatal(err)
			}
			movedFile := filepath.Join(moved, "file")
			if replaced == "parent" {
				movedFile = filepath.Join(moved, "child", "file")
			}
			if err := os.Link(movedFile, path); err != nil {
				t.Fatal(err)
			}
			file, err := d.OpenFile(node)
			if file != nil {
				_ = file.Close()
			}
			if err == nil {
				t.Fatal("OpenFile() accepted hard link through a replaced ancestor")
			}
		})
	}
}

func TestDirectoryRejectsReplacedAncestorWithOriginalDescendants(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	mustMkdir(t, parent)
	mustMkdir(t, child)
	path := filepath.Join(child, "file")
	mustWrite(t, path, "old")
	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	directory := d.Root().Children()[0].Children()[0]
	node := directory.Children()[0]
	if err := d.VerifyDirectory(directory); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(root, "moved")
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, parent)
	if err := os.Rename(filepath.Join(moved, "child"), child); err != nil {
		t.Fatal(err)
	}
	if err := d.VerifyDirectory(directory); err == nil {
		t.Error("VerifyDirectory() accepted original directory through a replaced ancestor")
	}
	file, err := d.OpenFile(node)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Error("OpenFile() accepted original file through a replaced ancestor")
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

func TestOpenDirectoryEntryLimitCountsExcludedEntries(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < MaxDirectoryEntries; i++ {
		mustWrite(t, filepath.Join(root, fmt.Sprintf(".%04d", i)), "")
	}
	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatalf("boundary rejected: %v", err)
	}
	_ = d.Close()
	mustWrite(t, filepath.Join(root, ".extra"), "")
	if _, err := OpenDirectory(root); err == nil {
		t.Fatal("entry count above limit accepted")
	}
}

func TestDirectoryWalkLimitsReadToRemainingEntries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".a", ".b", ".c"} {
		mustWrite(t, filepath.Join(root, name), "")
	}
	dir, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	files, entries := 0, MaxDirectoryEntries-1
	d := &Directory{}
	if err := d.walk(dir, &Node{}, 0, &files, &entries, newResourceID); err == nil {
		t.Fatal("entry count above remaining limit accepted")
	}
	unread, err := dir.ReadDir(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 {
		t.Fatalf("unread entries = %d, want 1", len(unread))
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
