package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/canta-9142/qshare/internal/receive"
)

const multipartOverhead int64 = 1 << 20

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}

	if s.uploadStore == nil {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		receive.MaxFileSize+multipartOverhead,
	)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError

		if errors.As(err, &maxBytesError) {
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	result, err := s.uploadStore.Save(
		r.Context(),
		header.Filename,
		file,
	)
	if err != nil {
		switch {
		case errors.Is(err, receive.ErrInvalidFilename):
			http.Error(w, "invalid filename", http.StatusBadRequest)

		case errors.Is(err, receive.ErrFileTooLarge):
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)

		default:
			http.Error(w, "failed to save upload", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}{
		Name: result.Name,
		Size: result.Size,
	})
}
