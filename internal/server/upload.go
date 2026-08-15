package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
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

	multipartReader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}

	file, err := nextUploadFile(multipartReader)
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

	filename, err := rawFilename(file.Header.Get("Content-Disposition"))
	if err != nil {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	result, err := s.uploadStore.Save(
		r.Context(),
		filename,
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

func nextUploadFile(reader *multipart.Reader) (*multipart.Part, error) {
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" && part.FileName() != "" {
			return part, nil
		}
		if _, err := io.Copy(io.Discard, part); err != nil {
			_ = part.Close()
			return nil, err
		}
		if err := part.Close(); err != nil {
			return nil, err
		}
	}
}

func rawFilename(contentDisposition string) (string, error) {
	mediaType, parameters, err := mime.ParseMediaType(contentDisposition)
	if err != nil || mediaType != "form-data" || parameters["filename"] == "" {
		return "", receive.ErrInvalidFilename
	}
	return parameters["filename"], nil
}
