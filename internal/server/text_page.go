package server

import (
	"net/http"
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

	setHTMLResponseHeaders(w, "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")

	_ = pageTemplates.ExecuteTemplate(w, "text.html", textPageData{Text: text.String()})
}
