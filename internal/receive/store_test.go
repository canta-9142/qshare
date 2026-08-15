package receive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestOpenStoreCreatesReceiveDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Downloads", "qshare")
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if store.dir != dir {
		t.Errorf("Store directory = %q, want %q", store.dir, dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("receive destination is not a directory")
	}
}

func TestOpenStoreRejectsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil {
		t.Fatal("OpenStore() error = nil, want error")
	}
}

func TestSaveStreamsFile(t *testing.T) {
	store := newTestStore(t)
	result, err := store.Save(context.Background(), "photo.jpg", strings.NewReader("image data"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if result.Name != "photo.jpg" || result.Size != int64(len("image data")) {
		t.Errorf("Save() result = %+v", result)
	}
	assertFileContent(t, filepath.Join(store.dir, result.Name), "image data")
	assertNoTemporaryFiles(t, store.dir)
}

func TestSaveRenamesCollisionsWithoutOverwriting(t *testing.T) {
	store := newTestStore(t)
	if err := os.WriteFile(filepath.Join(store.dir, "photo.jpg"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.dir, "photo (1).jpg"), []byte("existing one"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Save(context.Background(), "photo.jpg", strings.NewReader("new"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if result.Name != "photo (2).jpg" {
		t.Errorf("Name = %q, want %q", result.Name, "photo (2).jpg")
	}
	assertFileContent(t, filepath.Join(store.dir, "photo.jpg"), "existing")
	assertFileContent(t, filepath.Join(store.dir, "photo (1).jpg"), "existing one")
	assertFileContent(t, filepath.Join(store.dir, result.Name), "new")
}

func TestCollisionName(t *testing.T) {
	tests := []struct {
		name     string
		sequence int
		want     string
	}{
		{name: "photo.jpg", sequence: 0, want: "photo.jpg"},
		{name: "photo.jpg", sequence: 1, want: "photo (1).jpg"},
		{name: "archive.tar.gz", sequence: 2, want: "archive.tar (2).gz"},
		{name: "README", sequence: 1, want: "README (1)"},
		{name: ".env", sequence: 1, want: ".env (1)"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%d", tt.name, tt.sequence), func(t *testing.T) {
			if got := collisionName(tt.name, tt.sequence); got != tt.want {
				t.Errorf("collisionName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSaveRejectsUnsafeFilenames(t *testing.T) {
	store := newTestStore(t)
	for _, name := range []string{"", ".", "..", "../secret", `dir\secret`, "bad\x00name"} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			_, err := store.Save(context.Background(), name, strings.NewReader("content"))
			if !errors.Is(err, ErrInvalidFilename) {
				t.Fatalf("Save() error = %v, want ErrInvalidFilename", err)
			}
		})
	}
	assertDirectoryEmpty(t, store.dir)
}

func TestSaveRejectsFileOverLimit(t *testing.T) {
	store := newTestStore(t)
	store.maxFileSize = 8
	_, err := store.Save(context.Background(), "large.bin", strings.NewReader("123456789"))
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Save() error = %v, want ErrFileTooLarge", err)
	}
	assertDirectoryEmpty(t, store.dir)
}

func TestSaveAcceptsFileAtLimit(t *testing.T) {
	store := newTestStore(t)
	store.maxFileSize = 8
	result, err := store.Save(context.Background(), "limit.bin", strings.NewReader("12345678"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if result.Size != store.maxFileSize {
		t.Errorf("Save() size = %d, want %d", result.Size, store.maxFileSize)
	}
	assertFileContent(t, filepath.Join(store.dir, result.Name), "12345678")
}

func TestSaveRemovesPartialFileAfterReadError(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Save(context.Background(), "broken.txt", io.MultiReader(
		strings.NewReader("partial"),
		errorReader{},
	))
	if err == nil {
		t.Fatal("Save() error = nil, want error")
	}
	assertDirectoryEmpty(t, store.dir)
}

func TestSaveHonorsCanceledContext(t *testing.T) {
	store := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.Save(ctx, "canceled.txt", strings.NewReader("content"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}
	assertDirectoryEmpty(t, store.dir)
}

func TestSaveConcurrentCollisions(t *testing.T) {
	store := newTestStore(t)
	const uploads = 8
	results := make(chan Result, uploads)
	errorsChannel := make(chan error, uploads)
	var group sync.WaitGroup

	for index := range uploads {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := store.Save(context.Background(), "same.txt", strings.NewReader(fmt.Sprintf("content %d", index)))
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	close(errorsChannel)

	for err := range errorsChannel {
		t.Errorf("Save() error = %v", err)
	}
	var names []string
	for result := range results {
		names = append(names, result.Name)
	}
	if len(names) != uploads {
		t.Fatalf("saved %d files, want %d", len(names), uploads)
	}
	sort.Strings(names)
	seen := make(map[string]bool, uploads)
	for _, name := range names {
		if seen[name] {
			t.Fatalf("duplicate saved name %q", name)
		}
		seen[name] = true
	}
	assertNoTemporaryFiles(t, store.dir)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return store
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(content) != want {
		t.Errorf("ReadFile(%q) = %q, want %q", path, content, want)
	}
}

func assertDirectoryEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("receive directory contains %v, want empty", entries)
	}
}

func assertNoTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".qshare-upload-") {
			t.Errorf("temporary upload file remains: %s", entry.Name())
		}
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
