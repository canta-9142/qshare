package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestBuildDirectoryPageData(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "shared-root")
	if err := os.MkdirAll(filepath.Join(rootPath, "nested", "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "nested", "file.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory, err := share.OpenDirectory(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	root := directory.Root()
	nested := root.Children()[0]
	empty, file := nested.Children()[0], nested.Children()[1]
	const token = "test-token"
	rootLink := directoryLinkData{Name: "shared-root", URL: "/s/" + token}
	nestedLink := directoryLinkData{Name: "nested", URL: "/b/" + token + "/" + string(nested.ID())}
	emptyLink := directoryLinkData{Name: "empty", URL: "/b/" + token + "/" + string(empty.ID())}
	for _, tt := range []struct {
		name string
		node *share.Node
		want directoryPageData
	}{
		{"root", root, directoryPageData{
			Name: "shared-root", Breadcrumbs: []directoryLinkData{
				{Name: "shared-root", URL: rootLink.URL, Current: true},
			},
			Directories: []directoryLinkData{nestedLink}, ArchiveURL: "/z/" + token,
		}},
		{"nested", nested, directoryPageData{
			Name: "nested", Breadcrumbs: []directoryLinkData{
				rootLink,
				{Name: "nested", URL: nestedLink.URL, Current: true},
			},
			Directories: []directoryLinkData{emptyLink},
			Files:       []directoryFileData{{Name: "file.txt", Size: "5 B", URL: "/d/" + token + "/" + string(file.ID())}},
			ArchiveURL:  "/z/" + token,
		}},
		{"empty", empty, directoryPageData{
			Name: "empty", Breadcrumbs: []directoryLinkData{
				rootLink, nestedLink,
				{Name: "empty", URL: emptyLink.URL, Current: true},
			},
			ArchiveURL: "/z/" + token, IsEmpty: true,
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildDirectoryPageData(token, tt.node); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildDirectoryPageData() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

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
	srv.ServeHTTP(recorder, req)
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
	if !strings.Contains(body, `aria-current="page">shared-root</span>`) {
		t.Fatal("page does not identify the current breadcrumb")
	}
	if !strings.Contains(body, "Download shared directory as ZIP") {
		t.Fatal("page does not accurately label the shared-root archive")
	}
	child := directory.Root().Children()[0]
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/b/"+sess.Token().String()+"/"+string(child.ID()), nil)
	srv.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("child status = %d", recorder.Code)
	}
}

func TestDirectoryPageDescribesEmptyDirectory(t *testing.T) {
	srv, sess, _ := newDirectoryTestServer(t, t.TempDir())
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/s/"+sess.Token().String(), nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "This directory is empty.") {
		t.Fatal("empty directory does not have an explicit empty state")
	}
}

func TestDirectoryPageRejectsUnauthorizedAndUnknownNodes(t *testing.T) {
	srv, sess, _ := newDirectoryTestServer(t, t.TempDir())
	for _, target := range []string{"/s/not-a-token", "/b/" + sess.Token().String() + "/unknown", "/b/" + sess.Token().String() + "/..%2fetc"} {
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s status = %d", target, recorder.Code)
		}
	}
}

func TestDirectoryRootAuthorization(t *testing.T) {
	srv, sess, _ := newDirectoryTestServer(t, t.TempDir())
	wrong := sess.Token()
	wrong[0] ^= 0xff
	for _, tt := range []struct {
		name   string
		token  string
		now    time.Time
		status int
	}{
		{"wrong", wrong.String(), sess.ExpiresAt().Add(-time.Second), http.StatusNotFound},
		{"before expiry", sess.Token().String(), sess.ExpiresAt().Add(-time.Nanosecond), http.StatusOK},
		{"expiry boundary", sess.Token().String(), sess.ExpiresAt(), http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv.now = func() time.Time { return tt.now }
			recorder := httptest.NewRecorder()
			srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/s/"+tt.token, nil))
			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}
		})
	}
}

func newDirectoryTestServer(t *testing.T, root string) (*handler, *session.Session, *share.Directory) {
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
	return NewSendDirectory(sess).(*handler), sess, directory
}
