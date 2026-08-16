package share

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCollectionPreservesOrderAndDuplicateNames(t *testing.T) {
	root := t.TempDir()
	var paths []string
	for i, content := range []string{"first", "second"} {
		dir := filepath.Join(root, fmt.Sprint(i))
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "same.txt")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	collection, err := OpenCollection(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = collection.Close() })
	resources := collection.Resources()
	if len(resources) != 2 || resources[0].Name() != "same.txt" || resources[1].Name() != "same.txt" {
		t.Fatalf("resources = %#v", resources)
	}
	if resources[0].ID() == resources[1].ID() {
		t.Fatal("duplicate names received the same ID")
	}
	for _, resource := range resources {
		if strings.Contains(string(resource.ID()), resource.Name()) || strings.Contains(string(resource.ID()), root) {
			t.Fatalf("resource ID %q exposes local metadata", resource.ID())
		}
		if got, ok := collection.Lookup(resource.ID()); !ok || got != resource.File() {
			t.Fatal("Lookup() failed")
		}
	}
	if _, ok := collection.Lookup("unknown"); ok {
		t.Fatal("Lookup() accepted unknown ID")
	}
}

func TestOpenCollectionBoundaries(t *testing.T) {
	called := 0
	opener := func(string) (*File, error) { called++; return nil, errors.New("unexpected") }
	id := func() (ResourceID, error) { return "id", nil }
	for _, paths := range [][]string{nil, make([]string, MaxFiles+1)} {
		if _, err := openCollection(paths, opener, id); err == nil {
			t.Fatal("openCollection() error = nil")
		}
	}
	if called != 0 {
		t.Fatalf("open called %d times", called)
	}
}

func TestOpenCollectionAcceptsMaximum(t *testing.T) {
	path := writeTestFile(t, "content")
	paths := make([]string, MaxFiles)
	for i := range paths {
		paths[i] = path
	}
	collection, err := OpenCollection(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(collection.Resources()) != MaxFiles {
		t.Fatalf("len = %d", len(collection.Resources()))
	}
	if err := collection.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCollectionCleansUpAfterIntermediateFailure(t *testing.T) {
	path := writeTestFile(t, "content")
	collection, err := openCollection([]string{path, "missing"}, Open, func() (ResourceID, error) { return "id", nil })
	if err == nil || collection != nil {
		t.Fatal("openCollection() did not fail")
	}
	// A successfully closed descriptor makes a second close fail. Use the same
	// deterministic construction to directly verify cleanup behavior.
	var opened *File
	opener := func(p string) (*File, error) {
		if p == "bad" {
			return nil, errors.New("bad")
		}
		var openErr error
		opened, openErr = Open(path)
		return opened, openErr
	}
	_, _ = openCollection([]string{"good", "bad"}, opener, func() (ResourceID, error) { return "id", nil })
	if err := opened.Close(); err == nil {
		t.Fatal("partially opened file was not closed")
	}
}

func TestOpenCollectionRejectsSymlink(t *testing.T) {
	path := writeTestFile(t, "content")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCollection([]string{link}); err == nil {
		t.Fatal("OpenCollection() accepted symlink")
	}
}
