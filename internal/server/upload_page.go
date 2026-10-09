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

func (s *receiveHandler) uploadPage(w http.ResponseWriter, r *http.Request) {
	setHTMLResponseHeaders(
		w,
		"default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; form-action 'self'",
	)

	data := uploadPageData{
		UploadURL:         "/u/" + s.session.Token().String(),
		TextURL:           "/t/" + s.session.Token().String(),
		MaxUploadSize:     receive.MaxFileSize,
		MaxUploadSizeText: formatFileSize(receive.MaxFileSize),
		MaxTextSize:       share.MaxTextSize,
		MaxTextSizeText:   formatFileSize(share.MaxTextSize),
	}

	if err := pageTemplates.ExecuteTemplate(w, "upload.html", data); err != nil {
		return
	}
}
