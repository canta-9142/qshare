package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/canta-9142/qshare/internal/share"
)

func (s *Server) submitText(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}
	if s.textSubmitter == nil {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, share.MaxTextSize)
	value, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "text too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid text", http.StatusBadRequest)
		return
	}

	text, err := share.NewText(value)
	if err != nil {
		switch {
		case errors.Is(err, share.ErrTextTooLarge):
			http.Error(w, "text too large", http.StatusRequestEntityTooLarge)
		case errors.Is(err, share.ErrTextInvalidUTF8):
			http.Error(w, "text must be valid UTF-8", http.StatusBadRequest)
		default:
			http.Error(w, "invalid text", http.StatusBadRequest)
		}
		return
	}

	if err := s.textSubmitter.Submit(r.Context(), text); err != nil {
		http.Error(w, "failed to process text", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNoContent)
}
