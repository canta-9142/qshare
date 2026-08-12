package server

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/canta-9142/qshare/internal/session"
)

type Server struct {
	session  *session.Session
	server   *http.Server
	listener net.Listener
	done     chan error
	now      func() time.Time
}

func New(s *session.Session) *Server {
	mux := http.NewServeMux()

	server := &Server{
		session: s,
		done:    make(chan error, 1),
		now:     time.Now,
	}

	mux.HandleFunc("GET /s/{token}", server.page)

	mux.HandleFunc("GET /d/{token}", server.download)
	mux.HandleFunc("HEAD /d/{token}", server.download)

	server.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
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

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}

	resource := s.session.Resource()

	w.Header().Set(
		"Content-Disposition",
		mime.FormatMediaType(
			"attachment",
			map[string]string{
				"filename": resource.Name(),
			},
		),
	)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeContent(w, r, resource.Name(), resource.ModTime(), resource.Reader())
}

func (s *Server) tokenFromRequest(r *http.Request) (session.Token, error) {
	raw := r.PathValue("token")
	return session.ParseToken(raw)
}

func (s *Server) Done() <-chan error {
	return s.done
}
