package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestDirectoryPageNavigationOrderingAndEscaping(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared-root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "z-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "a-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "x<script>.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, sess, directory := newDirectoryTestServer(t, root)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil)
	srv.mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "<script>") || !strings.Contains(body, "x&lt;script&gt;.txt") {
		t.Fatalf("escaping failed: %s", body)
	}
	if strings.Index(body, "a-dir") > strings.Index(body, "z-dir") || strings.Index(body, "z-dir") > strings.Index(body, "x&lt;script&gt;") {
		t.Fatalf("ordering failed: %s", body)
	}
	if strings.Contains(body, root) {
		t.Fatal("page disclosed absolute path")
	}
	child := directory.Root().Children()[0]
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/b/"+sess.Token().String()+"/"+string(child.ID()), nil)
	srv.mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("child status = %d", recorder.Code)
	}
}

func TestDirectoryPageRejectsUnauthorizedAndUnknownNodes(t *testing.T) {
	srv, sess, _ := newDirectoryTestServer(t, t.TempDir())
	for _, target := range []string{"/s/not-a-token", "/b/" + sess.Token().String() + "/unknown", "/b/" + sess.Token().String() + "/..%2fetc"} {
		recorder := httptest.NewRecorder()
		srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s status = %d", target, recorder.Code)
		}
	}
}

func newDirectoryTestServer(t *testing.T, root string) (*Server, *session.Session, *share.Directory) {
	t.Helper()
	directory, err := share.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	sess, err := session.NewSendDirectory(directory, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return NewSendDirectory(sess), sess, directory
}
