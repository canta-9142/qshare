package server

import (
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

	downloadURL := "/d/" + sess.Token().String()
	if !strings.Contains(html, downloadURL) {
		t.Error("page does not contain authenticated download URL")
	}
}
