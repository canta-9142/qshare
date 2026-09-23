package share

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenPaths(t *testing.T) {
	dir := t.TempDir()
	first := writeTestFile(t, "first")
	second := writeTestFile(t, "second")
	missing := filepath.Join(dir, "missing")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	maximum := make([]string, MaxFiles)
	for i := range maximum {
		maximum[i] = first
	}
	tests := []struct {
		name      string
		paths     []string
		contents  []string
		directory bool
		wantError bool
		invalid   bool
	}{
		{name: "file", paths: []string{first}},
		{name: "ordered files", paths: []string{second, first}, contents: []string{"second", "first"}},
		{name: "maximum files", paths: maximum},
		{name: "directory", paths: []string{dir}, directory: true},
		{name: "empty", wantError: true, invalid: true},
		{name: "too many", paths: append(maximum, first), wantError: true, invalid: true},
		{name: "directory and file", paths: []string{dir, first}, wantError: true, invalid: true},
		{name: "file and directory", paths: []string{first, dir}, wantError: true, invalid: true},
		{name: "directories", paths: []string{dir, dir}, wantError: true, invalid: true},
		{name: "directory and missing", paths: []string{dir, missing}, wantError: true, invalid: true},
		{name: "missing and directory", paths: []string{missing, dir}, wantError: true, invalid: true},
		{name: "missing", paths: []string{missing}, wantError: true},
		{name: "partial files", paths: []string{first, missing}, wantError: true},
		{name: "directory symlink", paths: []string{link}, wantError: true},
		{name: "symlink and file", paths: []string{link, first}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, directory, err := OpenPaths(tt.paths)
			if files != nil {
				t.Cleanup(func() {
					if err := files.Close(); err != nil {
						t.Error(err)
					}
				})
			}
			if directory != nil {
				t.Cleanup(func() {
					if err := directory.Close(); err != nil {
						t.Error(err)
					}
				})
			}
			if (err != nil) != tt.wantError || errors.Is(err, ErrInvalidSelection) != tt.invalid {
				t.Fatalf("OpenPaths() error = %v, want error=%v invalid=%v", err, tt.wantError, tt.invalid)
			}
			if tt.wantError {
				if files != nil || directory != nil {
					t.Fatal("failed selection returned resources")
				}
				return
			}
			if (directory != nil) != tt.directory || (files != nil) == tt.directory {
				t.Fatalf("files=%v directory=%v, want directory=%v", files, directory, tt.directory)
			}
			if files != nil {
				resources := files.Resources()
				if len(resources) != len(tt.paths) {
					t.Fatalf("got %d files, want %d", len(resources), len(tt.paths))
				}
				for i, want := range tt.contents {
					got, err := io.ReadAll(resources[i].File().Reader())
					if err != nil || string(got) != want {
						t.Fatalf("file %d = %q, error=%v, want %q", i, got, err, want)
					}
				}
			}
		})
	}
}
