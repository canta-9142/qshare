package server

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/share"
)

//go:embed web/common.html web/upload.html
var uploadWebFiles embed.FS

var uploadPageTemplate = template.Must(
	template.ParseFS(uploadWebFiles, "web/common.html", "web/upload.html"),
)

type uploadPageData struct {
	UploadURL         string
	TextURL           string
	MaxUploadSize     int64
	MaxUploadSizeText string
	MaxTextSize       int
	MaxTextSizeText   string
}

func (s *Server) uploadPage(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}

	setHTMLResponseHeaders(
		w,
		"default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; form-action 'self'",
	)

	data := uploadPageData{
		UploadURL:         "/u/" + token.String(),
		TextURL:           "/t/" + token.String(),
		MaxUploadSize:     receive.MaxFileSize,
		MaxUploadSizeText: formatFileSize(receive.MaxFileSize),
		MaxTextSize:       share.MaxTextSize,
		MaxTextSizeText:   formatFileSize(share.MaxTextSize),
	}

	if err := uploadPageTemplate.ExecuteTemplate(w, "upload.html", data); err != nil {
		return
	}
}
