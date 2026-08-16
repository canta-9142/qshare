package server

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
)

func (s *Server) archive(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="qshare.zip"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	zw := zip.NewWriter(w)
	used := make(map[string]struct{})
	for _, resource := range s.session.Resources().Resources() {
		if err := r.Context().Err(); err != nil {
			_ = zw.Close()
			return
		}
		header := &zip.FileHeader{Name: uniqueArchiveName(resource.Name(), used), Method: zip.Deflate}
		header.SetModTime(resource.File().ModTime())
		entry, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return
		}
		if err := copyWithContext(r.Context(), entry, resource.File().Reader()); err != nil {
			_ = zw.Close()
			return
		}
	}
	_ = zw.Close()
}

func uniqueArchiveName(name string, used map[string]struct{}) string {
	name = filepath.Base(name)
	if _, exists := used[name]; !exists {
		used[name] = struct{}{}
		return name
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			if _, err := dst.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}
