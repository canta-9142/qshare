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
	assertArchiveAborts(t, func() { srv.ServeHTTP(recorder, req) })
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
	srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
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

func TestDirectoryArchiveSanitizesRootAndChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), `C:\shared`)
	for _, dir := range []string{root, filepath.Join(root, `a\b`), filepath.Join(root, "a_b.")} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"A_B", "a_b./file.txt", `a\b/x\..\outside.txt`, `a\b/a\b.txt`, `a\b/a_b.txt`} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	srv, sess, _ := newDirectoryTestServer(t, root)
	response := httptest.NewRecorder()
	srv.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"C__shared/": "", "C__shared/a_b/": "",
		"C__shared/a_b/x_.._outside.txt": `a\b/x\..\outside.txt`,
		"C__shared/a_b/a_b.txt":          `a\b/a\b.txt`,
		"C__shared/a_b/a_b (1).txt":      `a\b/a_b.txt`,
		"C__shared/a_b (1)/":             "",
		"C__shared/a_b (1)/file.txt":     "a_b./file.txt",
		"C__shared/A_B (2)":              "A_B",
	}
	if len(reader.File) != len(want) {
		t.Fatalf("entries = %d, want %d", len(reader.File), len(want))
	}
	for _, file := range reader.File {
		content, ok := want[file.Name]
		if !ok {
			t.Errorf("unexpected entry %q", file.Name)
			continue
		}
		delete(want, file.Name)
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != content {
			t.Errorf("entry %q content = %q, want %q", file.Name, body, content)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing entries: %v", want)
	}
}

func TestDirectoryDownloadAndArchiveRejectReplacedParentWithHardLink(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, directory := newDirectoryTestServer(t, root)
	node := directory.Root().Children()[0].Children()[0]
	if err := os.Rename(dir, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "moved", "file"), filepath.Join(dir, "file")); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		recorder := httptest.NewRecorder()
		target := "/d/" + sess.Token().String() + "/" + string(node.ID())
		srv.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s download status = %d, want 404", method, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	assertArchiveAborts(t, func() {
		srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	})
	if _, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len())); err == nil {
		t.Fatal("failed archive was finalized")
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
	srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
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
