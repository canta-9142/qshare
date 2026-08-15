package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestSubmitTextWaitsForSuccessfulProcessing(t *testing.T) {
	release := make(chan struct{})
	received := make(chan string, 1)
	server, sess := newTextReceiveTestServer(t, textSubmitterFunc(func(_ context.Context, text share.Text) error {
		received <- text.String()
		<-release
		return nil
	}))

	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.server.Handler.ServeHTTP(
			response,
			httptest.NewRequest(http.MethodPost, "/t/"+sess.Token().String(), strings.NewReader("hello, 世界")),
		)
		close(done)
	}()

	if got := <-received; got != "hello, 世界" {
		t.Fatalf("submitted text = %q", got)
	}
	select {
	case <-done:
		t.Fatal("handler returned before processing completed")
	default:
	}
	close(release)
	<-done

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestSubmitTextAcceptsExactSizeLimit(t *testing.T) {
	calls := 0
	server, sess := newTextReceiveTestServer(t, textSubmitterFunc(func(_ context.Context, text share.Text) error {
		calls++
		if text.Size() != share.MaxTextSize {
			t.Errorf("text size = %d, want %d", text.Size(), share.MaxTextSize)
		}
		return nil
	}))
	response := submitTextRequest(server, sess, strings.Repeat("x", share.MaxTextSize))
	if response.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("status = %d, calls = %d; want 204, 1", response.Code, calls)
	}
}

func TestSubmitTextRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{name: "over size limit", body: strings.Repeat("x", share.MaxTextSize+1), status: http.StatusRequestEntityTooLarge},
		{name: "invalid UTF-8", body: string([]byte{0xff}), status: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server, sess := newTextReceiveTestServer(t, textSubmitterFunc(func(context.Context, share.Text) error {
				calls++
				return nil
			}))
			response := submitTextRequest(server, sess, tt.body)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d", response.Code, tt.status)
			}
			if calls != 0 {
				t.Fatalf("Submit() calls = %d, want 0", calls)
			}
		})
	}
}

func TestSubmitTextRejectsUnauthorizedAndExpiredRequests(t *testing.T) {
	calls := 0
	server, sess := newTextReceiveTestServer(t, textSubmitterFunc(func(context.Context, share.Text) error {
		calls++
		return nil
	}))
	other := sess.Token()
	other[0] ^= 0xff
	for _, path := range []string{"/t/not-a-token", "/t/" + other.String()} {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader("secret")))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}

	server.now = sess.ExpiresAt
	response := submitTextRequest(server, sess, "secret")
	if response.Code != http.StatusNotFound {
		t.Errorf("expired status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if calls != 0 {
		t.Errorf("Submit() calls = %d, want 0", calls)
	}
}

func TestSubmitTextReportsFailureAndAcceptsLaterSubmission(t *testing.T) {
	want := errors.New("sink failed")
	calls := 0
	server, sess := newTextReceiveTestServer(t, textSubmitterFunc(func(context.Context, share.Text) error {
		calls++
		if calls == 1 {
			return want
		}
		return nil
	}))
	if response := submitTextRequest(server, sess, "first"); response.Code != http.StatusInternalServerError {
		t.Fatalf("first status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if response := submitTextRequest(server, sess, "second"); response.Code != http.StatusNoContent {
		t.Fatalf("second status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestSubmitTextRejectsUnsupportedMethod(t *testing.T) {
	server, sess := newTextReceiveTestServer(t, textSubmitterFunc(nil))
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/t/"+sess.Token().String(), nil),
	)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

type textSubmitterFunc func(context.Context, share.Text) error

func (function textSubmitterFunc) Submit(ctx context.Context, text share.Text) error {
	return function(ctx, text)
}

func newTextReceiveTestServer(t *testing.T, submitter textSubmitter) (*Server, *session.Session) {
	t.Helper()
	sess, err := session.NewReceive(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return NewReceive(sess, uploadStoreFunc(nil), submitter), sess
}

func submitTextRequest(server *Server, sess *session.Session, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/t/"+sess.Token().String(), strings.NewReader(body)),
	)
	return response
}
