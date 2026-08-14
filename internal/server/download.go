package server

import (
	"mime"
	"net/http"
)

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
