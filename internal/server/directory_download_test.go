package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDirectoryDownloadGETHEADRangeAndRetry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, directory := newDirectoryTestServer(t, root)
	node := directory.Root().Children()[0]
	target := "/d/" + sess.Token().String() + "/" + string(node.ID())
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		recorder := httptest.NewRecorder()
		srv.mux.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", method, recorder.Code)
		}
		if method == http.MethodGet && recorder.Body.String() != "0123456789" {
			t.Fatalf("body = %q", recorder.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Range", "bytes=2-4")
	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "234" {
		t.Fatalf("range = %d %q", recorder.Code, recorder.Body.String())
	}
	for i := 0; i < 2; i++ {
		recorder = httptest.NewRecorder()
		srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Fatal("retry failed")
		}
	}
}

func TestDirectoryDownloadAllowsSameObjectContentChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, directory := newDirectoryTestServer(t, root)
	node := directory.Root().Children()[0]
	if err := os.WriteFile(path, []byte("new-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/d/"+sess.Token().String()+"/"+string(node.ID()), nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "new-content" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestDirectoryDownloadRejectsReplacementSymlinkAndUnknownID(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, directory := newDirectoryTestServer(t, root)
	node := directory.Root().Children()[0]
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", path); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{string(node.ID()), "unknown"} {
		recorder := httptest.NewRecorder()
		srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/d/"+sess.Token().String()+"/"+id, nil))
		if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), "root:") {
			t.Fatalf("id %q response = %d", id, recorder.Code)
		}
	}
}

func TestDirectoryDownloadConcurrent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, directory := newDirectoryTestServer(t, root)
	node := directory.Root().Children()[0]
	target := "/d/" + sess.Token().String() + "/" + string(node.ID())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
			if recorder.Code != http.StatusOK || recorder.Body.String() != "content" {
				t.Errorf("response = %d %q", recorder.Code, recorder.Body.String())
			}
		}()
	}
	wg.Wait()
}
