package server

import (
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

func (s *fileHandler) download(w http.ResponseWriter, r *http.Request) {
	resource, ok := s.files.Lookup(share.ResourceID(r.PathValue("resource")))
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
