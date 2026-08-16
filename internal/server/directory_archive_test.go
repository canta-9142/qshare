package server

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryArchiveCancellationStopsGeneration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large"), bytes.Repeat([]byte("x"), 2<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, _ := newDirectoryTestServer(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, req)
	if recorder.Body.Len() > 1024 {
		t.Fatalf("cancelled archive wrote %d bytes", recorder.Body.Len())
	}
}

func TestDirectoryArchivePreservesHierarchyOrderAndEmptyDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared")
	for _, dir := range []string{root, filepath.Join(root, "a-empty"), filepath.Join(root, "b-dir")} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "b-dir", "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.txt"), []byte("z"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, _ := newDirectoryTestServer(t, root)
	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"shared/", "shared/a-empty/", "shared/b-dir/", "shared/b-dir/file.txt", "shared/z.txt"}
	if len(zr.File) != len(want) {
		t.Fatalf("entries = %d", len(zr.File))
	}
	for i, file := range zr.File {
		if file.Name != want[i] {
			t.Errorf("entry %d = %q, want %q", i, file.Name, want[i])
		}
		if filepath.IsAbs(file.Name) || strings.Contains(file.Name, "../") {
			t.Errorf("unsafe entry %q", file.Name)
		}
	}
	rc, err := zr.File[3].Open()
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(content) != "content" {
		t.Fatalf("content = %q", content)
	}
}

func TestDirectoryArchiveRejectsUnauthorizedAndChangedTree(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, _ := newDirectoryTestServer(t, root)
	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/z/not-a-token", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unauthorized status = %d", recorder.Code)
	}
	if err := os.Rename(dir, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	zr, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	if err == nil && len(zr.File) > 1 {
		t.Fatal("changed tree was included in archive")
	}
}

func TestDirectoryArchiveUsesCurrentSameObjectContents(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, _ := newDirectoryTestServer(t, root)
	if err := os.WriteFile(file, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	zr, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	rc, _ := zr.File[1].Open()
	content, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(content) != "new" {
		t.Fatalf("content = %q", content)
	}
}
