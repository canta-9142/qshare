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

func TestDirectoryPageRejectsUnknownNodes(t *testing.T) {
	srv, sess, _ := newDirectoryTestServer(t, t.TempDir())
	for _, target := range []string{"/b/" + sess.Token().String() + "/unknown", "/b/" + sess.Token().String() + "/..%2fetc"} {
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s status = %d", target, recorder.Code)
		}
	}
}

func TestDirectoryRejectsAnotherHandlersNodes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, firstSession, firstTree := newDirectoryTestServer(t, root)
	_, secondSession, secondTree := newDirectoryTestServer(t, root)
	for _, path := range []string{
		"/b/" + firstSession.Token().String() + "/" + string(secondTree.Root().ID()),
		"/d/" + firstSession.Token().String() + "/" + string(secondTree.Root().Children()[0].ID()),
		"/b/" + secondSession.Token().String() + "/" + string(firstTree.Root().ID()),
		"/d/" + secondSession.Token().String() + "/" + string(firstTree.Root().Children()[0].ID()),
		"/d/" + secondSession.Token().String() + "/" + string(secondTree.Root().Children()[0].ID()),
	} {
		response := httptest.NewRecorder()
		first.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	}
}

func newDirectoryTestServer(t *testing.T, root string) (*directoryHandler, *session.Session, *share.Directory) {
	t.Helper()
	directory, err := share.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	sess, err := session.New(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return NewSendDirectory(sess, directory).(*directoryHandler), sess, directory
}
