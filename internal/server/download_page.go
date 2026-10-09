package server

import (
	"fmt"
	"net/http"
)

type downloadPageData struct {
	Files      []downloadFileData
	ArchiveURL string
}

type downloadFileData struct {
	Name string
	Size string
	URL  string
}

func (s *fileHandler) downloadPage(w http.ResponseWriter, r *http.Request) {
	setHTMLResponseHeaders(w, "default-src 'none'; style-src 'unsafe-inline'")

	data := downloadPageData{ArchiveURL: "/z/" + s.session.Token().String()}
	for _, resource := range s.files.Resources() {
		data.Files = append(data.Files, downloadFileData{
			Name: resource.Name(),
			Size: formatFileSize(resource.Size()),
			URL:  "/d/" + s.session.Token().String() + "/" + string(resource.ID()),
		})
	}

	if err := pageTemplates.ExecuteTemplate(w, "download.html", data); err != nil {
		return
	}
}

func formatFileSize(size int64) string {
	const unit = 1024

	switch {
	case size < unit:
		return fmt.Sprintf("%d B", size)
	case size < unit*unit:
		return fmt.Sprintf("%.1f KiB", float64(size)/unit)
	case size < unit*unit*unit:
		return fmt.Sprintf("%.1f MiB", float64(size)/(unit*unit))
	default:
		return fmt.Sprintf("%.1f GiB", float64(size)/(unit*unit*unit))
	}
}
