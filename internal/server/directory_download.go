package server

import (
	"mime"
	"net/http"

	"github.com/canta-9142/qshare/internal/share"
)

func (s *Server) directoryDownload(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	node, ok := s.session.ResolveNode(token, share.ResourceID(r.PathValue("resource")), s.now())
	if !ok || node.Kind() != share.NodeFile {
		http.NotFound(w, r)
		return
	}
	file, err := s.session.Directory().OpenFile(node)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name()}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, file.Name(), file.ModTime(), file.Reader())
}
