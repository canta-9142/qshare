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
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
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

func TestSafeArchiveName(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`..\outside.txt`, ".._outside.txt"},
		{"/outside.txt", "_outside.txt"},
		{`C:\outside.txt`, "C__outside.txt"},
		{`\\server\share`, "__server_share"},
		{"", "_"}, {".", "_"}, {"..", "_"}, {"a\x00b", "a_b"},
		{"ordinary.txt", "ordinary.txt"},
		{".. ", "_"}, {"a.txt. ", "a.txt"},
		{"CON", "_CON"}, {"nul.txt", "_nul.txt"},
		{"con .txt", "_con .txt"}, {"CONIN$", "_CONIN$"}, {"CONOUT$", "_CONOUT$"},
		{"COM1.txt", "_COM1.txt"}, {"lpt9", "_lpt9"}, {"COM¹", "_COM¹"},
		{"COM0", "COM0"}, {"COM10.txt", "COM10.txt"}, {"COMLPT1", "COMLPT1"},
	} {
		if got := safeArchiveName(tc.input); got != tc.want {
			t.Errorf("safeArchiveName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestUniqueArchiveNameHandlesExtractionCollisions(t *testing.T) {
	used := make(map[string]struct{})
	for _, tc := range []struct{ input, want string }{
		{"A.txt", "A.txt"}, {"a.txt", "a (1).txt"},
		{"a.txt. ", "a (2).txt"}, {"A (1).txt", "A (1) (1).txt"},
		{"CON.txt", "_CON.txt"}, {"_con.txt", "_con (1).txt"},
		{".. ", "_"}, {"_", "_ (1)"},
	} {
		if got := uniqueArchiveName(tc.input, used); got != tc.want {
			t.Errorf("uniqueArchiveName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestArchiveSanitizesBeforeResolvingCollisions(t *testing.T) {
	server, sess := newMultiFileTestServer(t, []string{`..\outside.txt`, `a\b.txt`, "a_b.txt"}, []string{"one", "two", "three"})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".._outside.txt", "a_b.txt", "a_b (1).txt"}
	if len(reader.File) != len(want) {
		t.Fatalf("entries = %d", len(reader.File))
	}
	for i, file := range reader.File {
		if file.Name != want[i] {
			t.Errorf("entry %d = %q, want %q", i, file.Name, want[i])
		}
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

func assertArchiveAborts(t *testing.T, serve func()) {
	t.Helper()
	defer func() {
		if got := recover(); got != http.ErrAbortHandler {
			t.Fatalf("panic = %v, want http.ErrAbortHandler", got)
		}
	}()
	serve()
}

func TestArchiveAbortsOnCancellation(t *testing.T) {
	server, sess := newMultiFileTestServer(t, []string{"file"}, []string{"content"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil).WithContext(ctx)
	assertArchiveAborts(t, func() { server.ServeHTTP(recorder, req) })
	if recorder.Body.Len() != 0 {
		t.Fatalf("cancelled archive wrote %d bytes", recorder.Body.Len())
	}
}

func TestArchivesAbortOnFinalizationFailure(t *testing.T) {
	files, fileSession := newMultiFileTestServer(t, []string{"file"}, []string{"content"})
	directory, directorySession, _ := newDirectoryTestServer(t, t.TempDir())
	for _, tc := range []struct {
		name    string
		handler http.Handler
		token   string
	}{
		{"files", files, fileSession.Token().String()},
		{"directory", directory, directorySession.Token().String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &failingArchiveResponse{ResponseRecorder: httptest.NewRecorder()}
			assertArchiveAborts(t, func() {
				tc.handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/z/"+tc.token, nil))
			})
			if writer.writes != 1 {
				t.Fatalf("writes = %d, want one buffered write during Close", writer.writes)
			}
		})
	}
}

type failingArchiveResponse struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *failingArchiveResponse) Write([]byte) (int, error) {
	w.writes++
	return 0, io.ErrClosedPipe
}

func TestArchiveFailuresInterruptHTTPTransfer(t *testing.T) {
	for _, mode := range []string{"files", "directory"} {
		t.Run(mode, func(t *testing.T) {
			var handler http.Handler
			var token string
			if mode == "files" {
				server, sess := newMultiFileTestServer(t, []string{"first", "second"}, []string{"one", "two"})
				if err := server.files.Resources()[1].File().Close(); err != nil {
					t.Fatal(err)
				}
				handler, token = server, sess.Token().String()
			} else {
				root := t.TempDir()
				file := filepath.Join(root, "file")
				if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
					t.Fatal(err)
				}
				server, sess, _ := newDirectoryTestServer(t, root)
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				handler, token = server, sess.Token().String()
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Commit the response to exercise failure after HTTP streaming starts.
				w.(http.Flusher).Flush()
				handler.ServeHTTP(w, r)
			}))
			defer server.Close()
			response, err := server.Client().Get(server.URL + "/z/" + token)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != io.ErrUnexpectedEOF {
				t.Fatalf("read error = %v, want unexpected EOF", err)
			}
			if _, err := zip.NewReader(bytes.NewReader(body), int64(len(body))); err == nil {
				t.Fatal("failed archive was finalized")
			}
		})
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
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/z/"+sess.Token().String(), nil))
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

func TestCopyWithContextDetectsShortWrite(t *testing.T) {
	err := copyWithContext(context.Background(), shortArchiveWriter{}, bytes.NewReader([]byte("content")))
	if err != io.ErrShortWrite {
		t.Fatalf("error = %v, want %v", err, io.ErrShortWrite)
	}
}

type shortArchiveWriter struct{}

func (shortArchiveWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type countArchiveReader struct{ calls int }

func (r *countArchiveReader) Read([]byte) (int, error) { r.calls++; return 0, io.EOF }

func newMultiFileTestServer(t *testing.T, names, contents []string) (*fileHandler, *session.Session) {
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
	sess, err := session.New(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return NewSendFile(sess, resources).(*fileHandler), sess
}
