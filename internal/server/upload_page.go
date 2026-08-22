package server

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/canta-9142/qshare/internal/receive"
)

//go:embed web/upload.html
var uploadWebFiles embed.FS

var uploadPageTemplate = template.Must(
	template.ParseFS(uploadWebFiles, "web/upload.html"),
)

type uploadPageData struct {
	UploadURL     string
	TextURL       string
	MaxUploadSize int64
}

func (s *Server) uploadPage(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set(
		"Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; form-action 'self'",
	)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	data := uploadPageData{
		UploadURL:     "/u/" + token.String(),
		TextURL:       "/t/" + token.String(),
		MaxUploadSize: receive.MaxFileSize,
	}

	if err := uploadPageTemplate.ExecuteTemplate(w, "upload.html", data); err != nil {
		return
	}
}
