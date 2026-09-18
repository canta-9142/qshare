package server

import (
	"net/http"
)

type textPageData struct {
	Text string
}

func (s *textHandler) textPage(w http.ResponseWriter, r *http.Request) {
	setHTMLResponseHeaders(w, "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")

	_ = pageTemplates.ExecuteTemplate(w, "text.html", textPageData{Text: s.text.String()})
}
