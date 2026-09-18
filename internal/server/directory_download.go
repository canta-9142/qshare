package server

import (
	"net/http"

	"github.com/canta-9142/qshare/internal/share"
)

func (s *directoryHandler) directoryDownload(w http.ResponseWriter, r *http.Request) {
	node, ok := s.directory.Lookup(share.ResourceID(r.PathValue("resource")))
	if !ok || node.Kind() != share.NodeFile {
		http.NotFound(w, r)
		return
	}
	file, err := s.directory.OpenFile(node)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	serveDownload(w, r, file.Name(), file.ModTime(), file.Reader())
}
