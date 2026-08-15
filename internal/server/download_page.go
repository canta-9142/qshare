package server

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed web/download.html
var webFiles embed.FS

var downloadPage = template.Must(
	template.ParseFS(webFiles, "web/download.html"),
)

type downloadPageData struct {
	Files []downloadFileData
}

type downloadFileData struct {
	Name string
	Size string
	URL  string
}

func (s *Server) downloadPage(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	data := downloadPageData{}
	for _, resource := range s.session.Resources().Resources() {
		data.Files = append(data.Files, downloadFileData{
			Name: resource.Name(),
			Size: formatFileSize(resource.Size()),
			URL:  "/d/" + token.String() + "/" + string(resource.ID()),
		})
	}

	if err := downloadPage.ExecuteTemplate(w, "download.html", data); err != nil {
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
