package server

import (
	"net/http"
	"time"

	"github.com/canta-9142/qshare/internal/session"
)

type Server struct {
	session *session.Session
	server  *http.Server
}

func New(s *session.Session) *Server {
	mux := http.NewServeMux()

	server := &Server{
		session: s,
	}

	mux.HandleFunc("GET /d/{token}", server.download)
	mux.HandleFunc("HEAD /d/{token}", server.download)

	server.server = &http.Server{
		Handler: mux,
	}

	return server
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, time.Now()) {
		http.NotFound(w, r)
		return
	}

	resource := s.session.Resource()

	http.ServeContent(w, r, resource.Name(), resource.ModTime(), resource.Reader())
}

func (s *Server) tokenFromRequest(r *http.Request) (session.Token, error) {
	raw := r.PathValue("token")
	return session.ParseToken(raw)
}
