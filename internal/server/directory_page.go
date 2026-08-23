package server

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/canta-9142/qshare/internal/share"
)

//go:embed web/directory.html
var directoryWebFiles embed.FS

var directoryTemplate = template.Must(template.ParseFS(directoryWebFiles, "web/directory.html"))

type directoryPageData struct {
	Name        string
	Breadcrumbs []directoryLinkData
	Directories []directoryLinkData
	Files       []directoryFileData
	ArchiveURL  string
	IsEmpty     bool
}

type directoryLinkData struct {
	Name    string
	URL     string
	Current bool
}
type directoryFileData struct{ Name, Size, URL string }

func (s *Server) directoryRoot(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) || s.session.Directory() == nil {
		http.NotFound(w, r)
		return
	}
	s.renderDirectory(w, token.String(), s.session.Directory().Root())
}

func (s *Server) directoryPage(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	node, ok := s.session.ResolveNode(token, share.ResourceID(r.PathValue("resource")), s.now())
	if !ok || node.Kind() != share.NodeDirectory {
		http.NotFound(w, r)
		return
	}
	s.renderDirectory(w, token.String(), node)
}

func (s *Server) renderDirectory(w http.ResponseWriter, token string, node *share.Node) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	data := directoryPageData{Name: node.Name(), ArchiveURL: "/z/" + token}
	var lineage []*share.Node
	for current := node; current != nil; current = current.Parent() {
		lineage = append(lineage, current)
	}
	for i := len(lineage) - 1; i >= 0; i-- {
		current := lineage[i]
		url := "/b/" + token + "/" + string(current.ID())
		if current.Parent() == nil {
			url = "/s/" + token
		}
		data.Breadcrumbs = append(data.Breadcrumbs, directoryLinkData{
			Name:    current.Name(),
			URL:     url,
			Current: current == node,
		})
	}
	for _, child := range node.Children() {
		if child.Kind() == share.NodeDirectory {
			data.Directories = append(data.Directories, directoryLinkData{Name: child.Name(), URL: "/b/" + token + "/" + string(child.ID())})
		} else {
			data.Files = append(data.Files, directoryFileData{Name: child.Name(), Size: formatFileSize(child.Size()), URL: "/d/" + token + "/" + string(child.ID())})
		}
	}
	data.IsEmpty = len(data.Directories) == 0 && len(data.Files) == 0
	_ = directoryTemplate.ExecuteTemplate(w, "directory.html", data)
}
