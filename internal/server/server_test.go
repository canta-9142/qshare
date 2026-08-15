package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestDownload(t *testing.T) {
	server, sess := newTestServer(t, "download content")

	request := httptest.NewRequest(http.MethodGet, "/d/"+sess.Token().String(), nil)
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)

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
	server, sess := newTestServer(t, "download content")
	path := "/d/" + sess.Token().String()
	for attempt := 0; attempt < 2; attempt++ {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != "download content" {
			t.Fatalf("attempt %d: status=%d body=%q", attempt+1, response.Code, response.Body.String())
		}
	}
}

func TestDownloadHeadAndRangeRequests(t *testing.T) {
	server, sess := newTestServer(t, "abcdef")
	url := "/d/" + sess.Token().String()

	t.Run("HEAD", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodHead, url, nil)
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, request)

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
		server.server.Handler.ServeHTTP(response, request)

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
		server.server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestedRangeNotSatisfiable)
		}
	})
}

func TestDownloadRejectsUnsupportedMethod(t *testing.T) {
	server, sess := newTestServer(t, "secret")
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/d/"+sess.Token().String(), nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	server, sess := newTestServer(t, "over tcp")
	addr, err := server.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	response, err := http.Get("http://" + addr.String() + "/d/" + sess.Token().String())
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "over tcp" {
		t.Fatalf("response status=%d body=%q err=%v", response.StatusCode, body, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := <-server.Done(); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	if _, err := http.Get("http://" + addr.String() + "/d/" + sess.Token().String()); err == nil {
		t.Fatal("GET after Shutdown() error = nil")
	}
}

func TestServerClose(t *testing.T) {
	server, _ := newTestServer(t, "content")
	if _, err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := <-server.Done(); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
}

func TestDownloadRejectsUnauthorizedRequests(t *testing.T) {
	server, sess := newTestServer(t, "secret content")
	otherToken := sess.Token()
	otherToken[0] ^= 0xff

	tests := []struct {
		name string
		path string
	}{
		{name: "malformed token", path: "/d/not-a-token"},
		{name: "different token", path: "/d/" + otherToken.String()},
		{name: "unknown route", path: "/unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()
			server.server.Handler.ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
			if body := response.Body.String(); body == "secret content" {
				t.Fatal("unauthorized response exposed shared content")
			}
		})
	}
}

func TestDownloadRejectsRequestAtExpirationBoundary(t *testing.T) {
	server, sess := newTestServer(t, "secret content")
	server.now = sess.ExpiresAt

	request := httptest.NewRequest(http.MethodGet, "/d/"+sess.Token().String(), nil)
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
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
			server.server.Handler.ServeHTTP(response, request)

			if response.Code == http.StatusOK || response.Code == http.StatusPartialContent {
				t.Fatalf("status = %d, want request rejected", response.Code)
			}
			if body := response.Body.String(); body == "secret content" {
				t.Fatal("traversal response exposed shared content")
			}
		})
	}
}

func newTestServer(t *testing.T, content string) (*Server, *session.Session) {
	return newNamedTestServer(t, "shared.txt", content)
}

func newNamedTestServer(t *testing.T, name, content string) (*Server, *session.Session) {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	resource, err := share.Open(path)
	if err != nil {
		t.Fatalf("share.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := resource.Close(); err != nil {
			t.Errorf("resource.Close() error = %v", err)
		}
	})

	sess, err := session.NewSend(resource, time.Hour)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}
	return NewSend(sess), sess
}
