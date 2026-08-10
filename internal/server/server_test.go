package server

import (
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
	t.Helper()

	path := filepath.Join(t.TempDir(), "shared.txt")
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

	sess, err := session.New(resource, time.Hour)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}
	return New(sess), sess
}
