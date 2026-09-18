package server

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

type uploadStore interface {
	Save(context.Context, string, io.Reader) (receive.Result, error)
}

type textSubmitter interface {
	Submit(context.Context, share.Text) error
}

// handler binds route authorization to one session. Mode handlers borrow their
// resources from app, which owns their lifetime and cleanup.
type handler struct {
	session *session.Session
	mux     *http.ServeMux
	now     func() time.Time
}

type fileHandler struct {
	*handler
	files *share.Collection
}

type directoryHandler struct {
	*handler
	directory *share.Directory
}

type textHandler struct {
	*handler
	text share.Text
}

type receiveHandler struct {
	*handler
	uploadStore          uploadStore
	textSubmitter        textSubmitter
	maxUploadRequestSize int64
}

func NewSendFile(sess *session.Session, files *share.Collection) http.Handler {
	server := &fileHandler{handler: newHandler(sess), files: files}
	server.handle("GET /s/{token}", server.downloadPage)
	server.handle("GET /d/{token}/{resource}", server.download)
	server.handle("GET /z/{token}", server.archive)
	return server
}

func NewSendDirectory(sess *session.Session, directory *share.Directory) http.Handler {
	server := &directoryHandler{handler: newHandler(sess), directory: directory}
	server.handle("GET /s/{token}", server.directoryRoot)
	server.handle("GET /b/{token}/{resource}", server.directoryPage)
	server.handle("GET /d/{token}/{resource}", server.directoryDownload)
	server.handle("GET /z/{token}", server.directoryArchive)
	return server
}

func NewSendText(sess *session.Session, text share.Text) http.Handler {
	server := &textHandler{handler: newHandler(sess), text: text}
	server.handle("GET /s/{token}", server.textPage)
	return server
}

func NewReceive(sess *session.Session, store *receive.Store, submitter *receive.TextProcessor) http.Handler {
	return newReceive(sess, store, submitter)
}

func newReceive(sess *session.Session, store uploadStore, submitter textSubmitter) *receiveHandler {
	server := &receiveHandler{
		handler:              newHandler(sess),
		uploadStore:          store,
		textSubmitter:        submitter,
		maxUploadRequestSize: receive.MaxFileSize + multipartOverhead,
	}
	server.handle("GET /s/{token}", server.uploadPage)
	server.handle("POST /u/{token}", server.upload)
	server.handle("POST /t/{token}", server.submitText)
	return server
}

func newHandler(sess *session.Session) *handler {
	return &handler{
		session: sess,
		mux:     http.NewServeMux(),
		now:     time.Now,
	}
}

// NewHTTPServer applies the transport limits without binding or starting it.
func NewHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func (s *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handle authenticates after ServeMux has populated path values and before any
// resource lookup or request body processing. Every protected route uses it.
func (s *handler) handle(pattern string, next http.HandlerFunc) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		token, err := session.ParseToken(r.PathValue("token"))
		if err != nil || !s.session.Authorize(token, s.now()) {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	})
}
