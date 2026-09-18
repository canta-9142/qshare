package server

import (
	"net/http"

	"github.com/canta-9142/qshare/internal/share"
)

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

func (s *directoryHandler) directoryRoot(w http.ResponseWriter, r *http.Request) {
	s.renderDirectory(w, s.session.Token().String(), s.directory.Root())
}

func (s *directoryHandler) directoryPage(w http.ResponseWriter, r *http.Request) {
	node, ok := s.directory.Lookup(share.ResourceID(r.PathValue("resource")))
	if !ok || node.Kind() != share.NodeDirectory {
		http.NotFound(w, r)
		return
	}
	s.renderDirectory(w, s.session.Token().String(), node)
}

func (s *directoryHandler) renderDirectory(w http.ResponseWriter, token string, node *share.Node) {
	setHTMLResponseHeaders(w, "default-src 'none'; style-src 'unsafe-inline'")
	_ = pageTemplates.ExecuteTemplate(w, "directory.html", buildDirectoryPageData(token, node))
}

func buildDirectoryPageData(token string, node *share.Node) directoryPageData {
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
	return data
}
