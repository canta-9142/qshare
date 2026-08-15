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
	"sync"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestArchivePreservesOrderContentAndMakesDuplicateNamesUnique(t *testing.T) {
	server, sess := newMultiFileTestServer(t, []string{"same.txt", "same.txt", "same (1).txt"}, []string{"one", "two", "three"})
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"same.txt", "same (1).txt", "same (1) (1).txt"}
	for i, file := range reader.File {
		if file.Name != wantNames[i] {
			t.Errorf("entry %d name = %q, want %q", i, file.Name, wantNames[i])
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != []string{"one", "two", "three"}[i] {
			t.Errorf("entry content = %q", body)
		}
	}
}

func TestArchiveRejectsUnauthorizedRequests(t *testing.T) {
	server, sess := newTestServer(t, "secret")
	wrong := sess.Token()
	wrong[0] ^= 0xff
	for _, token := range []string{"invalid", wrong.String()} {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+token, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d", response.Code)
		}
	}
	server.now = sess.ExpiresAt
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expired status = %d", response.Code)
	}
}

func TestCopyWithContextStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &countArchiveReader{}
	if err := copyWithContext(ctx, io.Discard, reader); err != context.Canceled {
		t.Fatalf("error = %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("reader called %d times", reader.calls)
	}
}

func TestArchiveSupportsConcurrentDownloads(t *testing.T) {
	server, sess := newMultiFileTestServer(t, []string{"one.txt", "two.txt"}, []string{"one", "two"})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
			if response.Code != http.StatusOK {
				t.Errorf("status = %d", response.Code)
			}
			if _, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len())); err != nil {
				t.Errorf("invalid ZIP: %v", err)
			}
		}()
	}
	wg.Wait()
}

type countArchiveReader struct{ calls int }

func (r *countArchiveReader) Read([]byte) (int, error) { r.calls++; return 0, io.EOF }

func newMultiFileTestServer(t *testing.T, names, contents []string) (*Server, *session.Session) {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, len(names))
	for i := range names {
		dir := filepath.Join(root, string(rune('a'+i)))
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		paths[i] = filepath.Join(dir, names[i])
		if err := os.WriteFile(paths[i], []byte(contents[i]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resources, err := share.OpenCollection(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resources.Close() })
	sess, err := session.NewSendFiles(resources, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return NewSendFile(sess), sess
}
