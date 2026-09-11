package server

import (
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	resource, ok := s.session.Resolve(token, share.ResourceID(r.PathValue("resource")), s.now())
	if !ok {
		http.NotFound(w, r)
		return
	}

	serveDownload(w, r, resource.Name(), resource.ModTime(), resource.Reader())
}

func serveDownload(w http.ResponseWriter, r *http.Request, name string, modTime time.Time, reader io.ReadSeeker) {
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeContent(w, r, name, modTime, reader)
}
