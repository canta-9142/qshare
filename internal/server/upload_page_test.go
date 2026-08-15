package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadPage(t *testing.T) {
	server, sess := newReceiveTestServer(t, uploadStoreFunc(nil))
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil),
	)

	result := response.Result()
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if uploadURL := "/u/" + sess.Token().String(); !strings.Contains(string(body), uploadURL) {
		t.Errorf("page does not contain authenticated upload URL %q", uploadURL)
	}
	if textURL := "/t/" + sess.Token().String(); !strings.Contains(string(body), textURL) {
		t.Errorf("page does not contain authenticated text URL %q", textURL)
	}
	if !strings.Contains(string(body), `id="text-form"`) {
		t.Error("page does not contain text submission form")
	}
	for name, want := range map[string]string{
		"Cache-Control":           "private, no-store",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; form-action 'self'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	} {
		if got := result.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestUploadPageRejectsUnauthorizedRequests(t *testing.T) {
	server, sess := newReceiveTestServer(t, uploadStoreFunc(nil))
	other := sess.Token()
	other[0] ^= 0xff

	for _, path := range []string{"/s/not-a-token", "/s/" + other.String()} {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}

	server.now = sess.ExpiresAt
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil),
	)
	if response.Code != http.StatusNotFound {
		t.Errorf("expired page status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
