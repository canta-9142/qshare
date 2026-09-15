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

type handler struct {
	session              *session.Session
	uploadStore          uploadStore
	textSubmitter        textSubmitter
	maxUploadRequestSize int64
	mux                  *http.ServeMux
	now                  func() time.Time
}

func NewSendFile(sess *session.Session) http.Handler {
	server := newHandler(sess)

	server.mux.HandleFunc("GET /s/{token}", server.downloadPage)
	server.mux.HandleFunc("GET /d/{token}/{resource}", server.download)
	server.mux.HandleFunc("HEAD /d/{token}/{resource}", server.download)
	server.mux.HandleFunc("GET /z/{token}", server.archive)

	return server
}

func NewSendDirectory(sess *session.Session) http.Handler {
	server := newHandler(sess)
	server.mux.HandleFunc("GET /s/{token}", server.directoryRoot)
	server.mux.HandleFunc("GET /b/{token}/{resource}", server.directoryPage)
	server.mux.HandleFunc("GET /d/{token}/{resource}", server.directoryDownload)
	server.mux.HandleFunc("HEAD /d/{token}/{resource}", server.directoryDownload)
	server.mux.HandleFunc("GET /z/{token}", server.directoryArchive)
	return server
}

func NewSendText(sess *session.Session) http.Handler {
	server := newHandler(sess)

	server.mux.HandleFunc("GET /s/{token}", server.textPage)

	return server
}

func NewReceive(sess *session.Session, store uploadStore, submitter textSubmitter) http.Handler {
	server := newHandler(sess)

	server.mux.HandleFunc("GET /s/{token}", server.uploadPage)
	server.mux.HandleFunc("POST /u/{token}", server.upload)
	server.mux.HandleFunc("POST /t/{token}", server.submitText)
	server.uploadStore = store
	server.textSubmitter = submitter
	server.maxUploadRequestSize = receive.MaxFileSize + multipartOverhead

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

func (s *handler) tokenFromRequest(r *http.Request) (session.Token, error) {
	raw := r.PathValue("token")
	return session.ParseToken(raw)
}

func (s *handler) authorizeRequest(w http.ResponseWriter, r *http.Request) (session.Token, bool) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return session.Token{}, false
	}
	return token, true
}
