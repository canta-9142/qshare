package server

import (
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadPage(t *testing.T) {
	server, sess := newTestServer(t, "download content")
	path := "/s/" + sess.Token().String()

	request := httptest.NewRequest(http.MethodGet, path, nil)
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

	html := string(body)

	if !strings.Contains(html, "shared.txt") {
		t.Error("page does not contain shared filename")
	}

	wantURL := downloadURL(sess)
	if !strings.Contains(html, wantURL) {
		t.Error("page does not contain authenticated download URL")
	}
	for name, want := range map[string]string{
		"Cache-Control":           "private, no-store",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	} {
		if got := result.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestDownloadPageRejectsUnauthorizedRequests(t *testing.T) {
	server, sess := newTestServer(t, "content")
	other := sess.Token()
	other[0] ^= 0xff
	for _, path := range []string{"/s/not-a-token", "/s/" + other.String()} {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, response.Code)
		}
	}
	server.now = sess.ExpiresAt
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil))
	if response.Code != http.StatusNotFound {
		t.Errorf("expired page status = %d, want 404", response.Code)
	}
}

func TestDownloadPageEscapesFileName(t *testing.T) {
	name := `<script>alert("x").txt`
	server, sess := newNamedTestServer(t, name, "content")
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil))
	body := response.Body.String()
	if strings.Contains(body, name) || !strings.Contains(body, html.EscapeString(name)) {
		t.Fatalf("page did not safely escape filename: %q", body)
	}
}

func TestDownloadPageListsDuplicateNamesInOrderWithoutLocalPaths(t *testing.T) {
	server, sess := newMultiFileTestServer(t, []string{"same.txt", "middle.txt", "same.txt"}, []string{"one", "two", "three"})
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil))
	body := response.Body.String()
	first := strings.Index(body, "same.txt")
	middle := strings.Index(body, "middle.txt")
	last := strings.LastIndex(body, "same.txt")
	if first < 0 || !(first < middle && middle < last) {
		t.Fatalf("files are not in CLI order: %q", body)
	}
	for _, resource := range sess.Resources().Resources() {
		url := "/d/" + sess.Token().String() + "/" + string(resource.ID())
		if !strings.Contains(body, url) {
			t.Errorf("page missing URL %q", url)
		}
	}
	if strings.Contains(body, "file://") || strings.Contains(body, "../") {
		t.Fatal("page exposes path-like local metadata")
	}
}
