package server

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed web/common.html web/text.html
var textWebFiles embed.FS

var textPageTemplate = template.Must(
	template.ParseFS(textWebFiles, "web/common.html", "web/text.html"),
)

type textPageData struct {
	Text string
}

func (s *Server) textPage(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}

	text, ok := s.session.Text()
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	_ = textPageTemplate.ExecuteTemplate(w, "text.html", textPageData{Text: text.String()})
}
