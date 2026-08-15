package server

import (
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestTextPageDisplaysEscapedTextAndCopyButton(t *testing.T) {
	value := `<script>alert("x")</script> & text`
	server, sess := newTextTestServer(t, value)
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil),
	)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if strings.Contains(body, value) || !strings.Contains(body, html.EscapeString(value)) {
		t.Fatalf("page did not safely escape text: %q", body)
	}
	if !strings.Contains(body, `id="copy"`) || !strings.Contains(body, "navigator.clipboard.writeText") {
		t.Fatal("page does not contain Copy button behavior")
	}
	for name, want := range map[string]string{
		"Content-Type":            "text/html; charset=utf-8",
		"Cache-Control":           "private, no-store",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	} {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestTextPageRejectsUnauthorizedAndExpiredRequests(t *testing.T) {
	server, sess := newTextTestServer(t, "secret")
	other := sess.Token()
	other[0] ^= 0xff

	for _, path := range []string{"/s/not-a-token", "/s/" + other.String()} {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
		if strings.Contains(response.Body.String(), "secret") {
			t.Errorf("%s: unauthorized response exposed text", path)
		}
	}

	server.now = sess.ExpiresAt
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil),
	)
	if response.Code != http.StatusNotFound {
		t.Errorf("expired request status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestTextPageRejectsUnsupportedMethod(t *testing.T) {
	server, sess := newTextTestServer(t, "secret")
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/s/"+sess.Token().String(), nil),
	)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func newTextTestServer(t *testing.T, value string) (*Server, *session.Session) {
	t.Helper()
	text, err := share.NewText([]byte(value))
	if err != nil {
		t.Fatalf("share.NewText() error = %v", err)
	}
	sess, err := session.NewSendText(text, time.Hour)
	if err != nil {
		t.Fatalf("session.NewSendText() error = %v", err)
	}
	return NewSendText(sess), sess
}
