package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestAllRoutesRequireTheirSession(t *testing.T) {
	files, _ := newTestServer(t, "secret")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory, _, tree := newDirectoryTestServer(t, root)
	text, _ := newTextTestServer(t, "secret")
	sess, err := session.New(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var saveCalls, submitCalls int
	receiver := newReceive(sess,
		uploadStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
			saveCalls++
			return receive.Result{}, nil
		}),
		textSubmitterFunc(func(context.Context, share.Text) error {
			submitCalls++
			return nil
		}),
	)
	other, err := session.New(time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	filePath := "/d/%s/" + string(files.files.Resources()[0].ID())
	directoryPath := "/b/%s/" + string(tree.Root().ID())
	directoryFilePath := "/d/%s/" + string(tree.Root().Children()[0].ID())
	for _, route := range []struct {
		mode         string
		h            *handler
		method, path string
		status       int
	}{
		{"files", files.handler, http.MethodGet, "/s/%s", http.StatusOK},
		{"files", files.handler, http.MethodHead, "/s/%s", http.StatusOK},
		{"files", files.handler, http.MethodGet, filePath, http.StatusOK},
		{"files", files.handler, http.MethodHead, filePath, http.StatusOK},
		{"files", files.handler, http.MethodGet, "/z/%s", http.StatusOK},
		{"files", files.handler, http.MethodHead, "/z/%s", http.StatusOK},
		{"directory", directory.handler, http.MethodGet, "/s/%s", http.StatusOK},
		{"directory", directory.handler, http.MethodHead, "/s/%s", http.StatusOK},
		{"directory", directory.handler, http.MethodGet, directoryPath, http.StatusOK},
		{"directory", directory.handler, http.MethodHead, directoryPath, http.StatusOK},
		{"directory", directory.handler, http.MethodGet, directoryFilePath, http.StatusOK},
		{"directory", directory.handler, http.MethodHead, directoryFilePath, http.StatusOK},
		{"directory", directory.handler, http.MethodGet, "/z/%s", http.StatusOK},
		{"directory", directory.handler, http.MethodHead, "/z/%s", http.StatusOK},
		{"text", text.handler, http.MethodGet, "/s/%s", http.StatusOK},
		{"text", text.handler, http.MethodHead, "/s/%s", http.StatusOK},
		{"receive", receiver.handler, http.MethodGet, "/s/%s", http.StatusOK},
		{"receive", receiver.handler, http.MethodHead, "/s/%s", http.StatusOK},
		{"receive", receiver.handler, http.MethodPost, "/u/%s", http.StatusBadRequest}, // Missing multipart header.
		{"receive", receiver.handler, http.MethodPost, "/t/%s", http.StatusNoContent},
	} {
		for _, auth := range []struct {
			name, token string
			now         time.Time
			status      int
		}{
			{"valid", route.h.session.Token().String(), route.h.session.ExpiresAt().Add(-time.Nanosecond), route.status},
			{"malformed", "not-a-token", time.Now(), http.StatusNotFound},
			{"another session", other.Token().String(), time.Now(), http.StatusNotFound},
			{"expired", route.h.session.Token().String(), route.h.session.ExpiresAt(), http.StatusNotFound},
		} {
			t.Run(route.mode+"/"+route.method+route.path+"/"+auth.name, func(t *testing.T) {
				saveCalls, submitCalls = 0, 0
				route.h.now = func() time.Time { return auth.now }
				body := &countArchiveReader{}
				response := httptest.NewRecorder()
				route.h.ServeHTTP(response, httptest.NewRequest(route.method, fmt.Sprintf(route.path, auth.token), body))
				if response.Code != auth.status {
					t.Fatalf("status = %d, want %d", response.Code, auth.status)
				}
				if auth.status == http.StatusNotFound {
					if body.calls != 0 || saveCalls != 0 || submitCalls != 0 {
						t.Fatal("unauthorized request read its body or invoked a receiver")
					}
					if strings.Contains(response.Body.String(), "secret") {
						t.Fatal("unauthorized response exposed shared content")
					}
				}
			})
		}
	}
}

func TestDownload(t *testing.T) {
	server, _ := newTestServer(t, "download content")

	request := httptest.NewRequest(http.MethodGet, downloadURL(server), nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	result := response.Result()
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got, want := string(body), "download content"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := result.Header.Get("Content-Disposition"); got != `attachment; filename=shared.txt` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if got := result.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := result.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}

func TestDownloadCanBeRetried(t *testing.T) {
	server, _ := newTestServer(t, "download content")
	path := downloadURL(server)
	for attempt := 0; attempt < 2; attempt++ {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != "download content" {
			t.Fatalf("attempt %d: status=%d body=%q", attempt+1, response.Code, response.Body.String())
		}
	}
}

func TestDownloadHeadAndRangeRequests(t *testing.T) {
	server, _ := newTestServer(t, "abcdef")
	url := downloadURL(server)

	t.Run("HEAD", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodHead, url, nil)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)

		result := response.Result()
		defer result.Body.Close()
		if result.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(result.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if len(body) != 0 {
			t.Errorf("HEAD body length = %d, want 0", len(body))
		}
		if got, want := result.ContentLength, int64(6); got != want {
			t.Errorf("Content-Length = %d, want %d", got, want)
		}
	})

	t.Run("Range", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request.Header.Set("Range", "bytes=2-4")
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)

		result := response.Result()
		defer result.Body.Close()
		if result.StatusCode != http.StatusPartialContent {
			t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusPartialContent)
		}
		body, err := io.ReadAll(result.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if got, want := string(body), "cde"; got != want {
			t.Errorf("body = %q, want %q", got, want)
		}
	})

	t.Run("invalid Range", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request.Header.Set("Range", "bytes=99-100")
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestedRangeNotSatisfiable)
		}
	})
}

func TestDownloadRejectsUnsupportedMethod(t *testing.T) {
	server, _ := newTestServer(t, "secret")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, downloadURL(server), nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestHTTPServerLimits(t *testing.T) {
	h, _ := newTestServer(t, "content")
	srv := NewHTTPServer(h)
	if srv.Handler != h || srv.ReadHeaderTimeout != 5*time.Second ||
		srv.IdleTimeout != 30*time.Second || srv.MaxHeaderBytes != 1<<20 {
		t.Fatalf("unexpected HTTP server configuration: %+v", srv)
	}
}

func TestDownloadRejectsUnknownResourceAndRoute(t *testing.T) {
	server, sess := newTestServer(t, "secret content")

	tests := []struct {
		name string
		path string
	}{
		{name: "missing resource", path: "/d/" + sess.Token().String()},
		{name: "unknown resource", path: "/d/" + sess.Token().String() + "/unknown"},
		{name: "unknown route", path: "/unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
			if body := response.Body.String(); body == "secret content" {
				t.Fatal("unauthorized response exposed shared content")
			}
		})
	}
}

func TestDownloadRejectsAnotherSessionsResourceID(t *testing.T) {
	server, sess := newTestServer(t, "first")
	other, _ := newNamedTestServer(t, "other.txt", "second")
	path := "/d/" + sess.Token().String() + "/" + string(other.files.Resources()[0].ID())
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestDownloadDoesNotServeTraversalPaths(t *testing.T) {
	server, _ := newTestServer(t, "secret content")

	paths := []string{
		"/d/../shared.txt",
		"/d/%2e%2e%2fshared.txt",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code == http.StatusOK || response.Code == http.StatusPartialContent {
				t.Fatalf("status = %d, want request rejected", response.Code)
			}
			if body := response.Body.String(); body == "secret content" {
				t.Fatal("traversal response exposed shared content")
			}
		})
	}
}

func newTestServer(t *testing.T, content string) (*fileHandler, *session.Session) {
	return newNamedTestServer(t, "shared.txt", content)
}

func newNamedTestServer(t *testing.T, name, content string) (*fileHandler, *session.Session) {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	resources, err := share.OpenCollection([]string{path})
	if err != nil {
		t.Fatalf("share.OpenCollection() error = %v", err)
	}
	t.Cleanup(func() { _ = resources.Close() })
	sess, err := session.New(time.Hour)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}
	return NewSendFile(sess, resources).(*fileHandler), sess
}

func downloadURL(server *fileHandler) string {
	return "/d/" + server.session.Token().String() + "/" + string(server.files.Resources()[0].ID())
}
