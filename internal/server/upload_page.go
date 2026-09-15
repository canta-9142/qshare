package server

import (
	"net/http"

	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/share"
)

type uploadPageData struct {
	UploadURL         string
	TextURL           string
	MaxUploadSize     int64
	MaxUploadSizeText string
	MaxTextSize       int
	MaxTextSizeText   string
}

func (s *handler) uploadPage(w http.ResponseWriter, r *http.Request) {
	token, ok := s.authorizeRequest(w, r)
	if !ok {
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

	if err := pageTemplates.ExecuteTemplate(w, "upload.html", data); err != nil {
		return
	}
}
