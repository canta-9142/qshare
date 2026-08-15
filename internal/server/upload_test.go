package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
)

func TestUploadSavesFile(t *testing.T) {
	var savedName string
	var savedContent string
	store := uploadStoreFunc(func(_ context.Context, name string, source io.Reader) (receive.Result, error) {
		content, err := io.ReadAll(source)
		if err != nil {
			return receive.Result{}, err
		}
		savedName = name
		savedContent = string(content)
		return receive.Result{Name: "photo (1).jpg", Size: int64(len(content))}, nil
	})
	server, sess := newReceiveTestServer(t, store)

	request := newUploadRequest(t, "/u/"+sess.Token().String(), "photo.jpg", "image data")
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %q", response.Code, http.StatusCreated, response.Body.String())
	}
	if savedName != "photo.jpg" || savedContent != "image data" {
		t.Errorf("store received name=%q content=%q", savedName, savedContent)
	}
	var result receive.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Name != "photo (1).jpg" || result.Size != int64(len("image data")) {
		t.Errorf("response = %+v", result)
	}
	for name, want := range map[string]string{
		"Content-Type":           "application/json",
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestUploadCanBeRepeatedInSameSession(t *testing.T) {
	calls := 0
	store := uploadStoreFunc(func(_ context.Context, _ string, source io.Reader) (receive.Result, error) {
		calls++
		_, _ = io.Copy(io.Discard, source)
		return receive.Result{Name: "file.txt", Size: 1}, nil
	})
	server, sess := newReceiveTestServer(t, store)
	path := "/u/" + sess.Token().String()

	for attempt := 0; attempt < 2; attempt++ {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, newUploadRequest(t, path, "file.txt", "x"))
		if response.Code != http.StatusCreated {
			t.Fatalf("attempt %d: status = %d, want %d", attempt+1, response.Code, http.StatusCreated)
		}
	}
	if calls != 2 {
		t.Errorf("Save() calls = %d, want 2", calls)
	}
}

func TestUploadRejectsUnauthorizedRequests(t *testing.T) {
	calls := 0
	store := uploadStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
		calls++
		return receive.Result{}, nil
	})
	server, sess := newReceiveTestServer(t, store)
	other := sess.Token()
	other[0] ^= 0xff

	for _, path := range []string{"/u/not-a-token", "/u/" + other.String()} {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, newUploadRequest(t, path, "file.txt", "secret"))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
	server.now = sess.ExpiresAt
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, newUploadRequest(t, "/u/"+sess.Token().String(), "file.txt", "secret"))
	if response.Code != http.StatusNotFound {
		t.Errorf("expired request status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if calls != 0 {
		t.Errorf("Save() calls = %d, want 0", calls)
	}
}

func TestUploadRejectsInvalidMultipartRequest(t *testing.T) {
	server, sess := newReceiveTestServer(t, uploadStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
		t.Fatal("Save() called for invalid request")
		return receive.Result{}, nil
	}))
	request := httptest.NewRequest(http.MethodPost, "/u/"+sess.Token().String(), strings.NewReader("not multipart"))
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestUploadRejectsMultipartWithoutFile(t *testing.T) {
	server, sess := newReceiveTestServer(t, uploadStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
		t.Fatal("Save() called for request without a file")
		return receive.Result{}, nil
	}))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("note", "not a file"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/u/"+sess.Token().String(), &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestRawFilenameRejectsInvalidContentDisposition(t *testing.T) {
	for _, value := range []string{
		"",
		"attachment; filename=file.txt",
		"form-data",
		`form-data; filename=""`,
		`form-data; filename="unterminated`,
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := rawFilename(value); !errors.Is(err, receive.ErrInvalidFilename) {
				t.Fatalf("rawFilename() error = %v, want ErrInvalidFilename", err)
			}
		})
	}
}

func TestUploadRejectsRequestOverLimitAndRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	store, err := receive.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	server, sess := newReceiveTestServer(t, store)
	request := newUploadRequest(t, "/u/"+sess.Token().String(), "large.bin", strings.Repeat("x", 256))
	server.maxUploadRequestSize = request.ContentLength - 64

	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body = %q", response.Code, http.StatusRequestEntityTooLarge, response.Body.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("receive directory contains %v, want empty", entries)
	}
}

func TestUploadAcceptsRequestAtLimit(t *testing.T) {
	store := uploadStoreFunc(func(_ context.Context, name string, source io.Reader) (receive.Result, error) {
		content, err := io.ReadAll(source)
		return receive.Result{Name: name, Size: int64(len(content))}, err
	})
	server, sess := newReceiveTestServer(t, store)
	request := newUploadRequest(t, "/u/"+sess.Token().String(), "limit.bin", "content")
	server.maxUploadRequestSize = request.ContentLength

	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %q", response.Code, http.StatusCreated, response.Body.String())
	}
}

func TestUploadCancellationRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	store, err := receive.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	server, sess := newReceiveTestServer(t, store)
	request := newUploadRequest(t, "/u/"+sess.Token().String(), "cancelled.txt", "content")
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("receive directory contains %v, want empty", entries)
	}
}

func TestUploadRejectsFilenameWithPathSeparator(t *testing.T) {
	var receivedNames []string
	store := uploadStoreFunc(func(_ context.Context, name string, _ io.Reader) (receive.Result, error) {
		receivedNames = append(receivedNames, name)
		return receive.Result{}, receive.ErrInvalidFilename
	})
	server, sess := newReceiveTestServer(t, store)

	for _, filename := range []string{"../secret.txt", `..\secret.txt`} {
		t.Run(filename, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.server.Handler.ServeHTTP(
				response,
				newUploadRequest(t, "/u/"+sess.Token().String(), filename, "secret"),
			)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
	wantNames := []string{"../secret.txt", `..\secret.txt`}
	if !slices.Equal(receivedNames, wantNames) {
		t.Errorf("Save() filenames = %q, want %q", receivedNames, wantNames)
	}
}

func TestUploadMapsStoreErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid filename", err: receive.ErrInvalidFilename, wantStatus: http.StatusBadRequest},
		{name: "file too large", err: receive.ErrFileTooLarge, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "storage failure", err: errors.New("disk failed"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, sess := newReceiveTestServer(t, uploadStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
				return receive.Result{}, tt.err
			}))
			response := httptest.NewRecorder()
			server.server.Handler.ServeHTTP(response, newUploadRequest(t, "/u/"+sess.Token().String(), "file.txt", "content"))
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
		})
	}
}

func TestReceiveServerRejectsUnsupportedRoutes(t *testing.T) {
	calls := 0
	server, sess := newReceiveTestServer(t, uploadStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
		calls++
		return receive.Result{}, nil
	}))
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/u/"+sess.Token().String(), nil),
		httptest.NewRequest(http.MethodGet, "/d/"+sess.Token().String(), nil),
		httptest.NewRequest(http.MethodPost, "/u/%2e%2e%2f"+sess.Token().String(), nil),
		httptest.NewRequest(http.MethodPost, "/u/%2e%2e/"+sess.Token().String(), nil),
	} {
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed && response.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404 or 405", request.Method, request.URL.Path, response.Code)
		}
	}
	if calls != 0 {
		t.Errorf("Save() calls = %d, want 0", calls)
	}
}

type uploadStoreFunc func(context.Context, string, io.Reader) (receive.Result, error)

func (function uploadStoreFunc) Save(ctx context.Context, name string, source io.Reader) (receive.Result, error) {
	return function(ctx, name, source)
}

func newReceiveTestServer(t *testing.T, store uploadStore) (*Server, *session.Session) {
	t.Helper()
	sess, err := session.NewReceive(time.Hour)
	if err != nil {
		t.Fatalf("session.NewReceive() error = %v", err)
	}
	return NewReceive(sess, store, nil), sess
}

func newUploadRequest(t *testing.T, path, filename, content string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatalf("write multipart content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
