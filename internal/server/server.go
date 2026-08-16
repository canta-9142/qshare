package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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

type Server struct {
	session              *session.Session
	uploadStore          uploadStore
	textSubmitter        textSubmitter
	maxUploadRequestSize int64
	server               *http.Server
	mux                  *http.ServeMux
	listener             net.Listener
	done                 chan error
	now                  func() time.Time
}

func NewSendFile(sess *session.Session) *Server {
	server := newServer(sess)

	server.mux.HandleFunc("GET /s/{token}", server.downloadPage)
	server.mux.HandleFunc("GET /d/{token}/{resource}", server.download)
	server.mux.HandleFunc("HEAD /d/{token}/{resource}", server.download)
	server.mux.HandleFunc("GET /z/{token}", server.archive)

	return server
}

func NewSendDirectory(sess *session.Session) *Server {
	server := newServer(sess)
	server.mux.HandleFunc("GET /s/{token}", server.directoryRoot)
	server.mux.HandleFunc("GET /b/{token}/{resource}", server.directoryPage)
	server.mux.HandleFunc("GET /d/{token}/{resource}", server.directoryDownload)
	server.mux.HandleFunc("HEAD /d/{token}/{resource}", server.directoryDownload)
	return server
}

func NewSendText(sess *session.Session) *Server {
	server := newServer(sess)

	server.mux.HandleFunc("GET /s/{token}", server.textPage)

	return server
}

func NewReceive(sess *session.Session, store uploadStore, submitter textSubmitter) *Server {
	server := newServer(sess)

	server.mux.HandleFunc("GET /s/{token}", server.uploadPage)
	server.mux.HandleFunc("POST /u/{token}", server.upload)
	server.mux.HandleFunc("POST /t/{token}", server.submitText)
	server.uploadStore = store
	server.textSubmitter = submitter
	server.maxUploadRequestSize = receive.MaxFileSize + multipartOverhead

	return server
}

func newServer(sess *session.Session) *Server {
	mux := http.NewServeMux()

	server := &Server{
		session: sess,
		mux:     mux,
		done:    make(chan error, 1),
		now:     time.Now,
	}

	server.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	return server
}

func (s *Server) Start(bindAddr string) (net.Addr, error) {
	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	s.listener = ln

	go func() {
		err := s.server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.done <- err
		close(s.done)
	}()

	return ln.Addr(), nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}

func (s *Server) Close() error {
	if err := s.server.Close(); err != nil {
		return fmt.Errorf("close HTTP server: %w", err)
	}
	return nil
}

func (s *Server) tokenFromRequest(r *http.Request) (session.Token, error) {
	raw := r.PathValue("token")
	return session.ParseToken(raw)
}

func (s *Server) Done() <-chan error {
	return s.done
}
