package server

import (
	"archive/zip"
	"context"
	"mime"
	"net/http"
	"path"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

func (s *Server) directoryArchive(w http.ResponseWriter, r *http.Request) {
	token, err := s.tokenFromRequest(r)
	if err != nil || !s.session.Authorize(token, s.now()) || s.session.Directory() == nil {
		http.NotFound(w, r)
		return
	}
	root := s.session.Directory().Root()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": root.Name() + ".zip"}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	zw := zip.NewWriter(w)
	if err := s.writeDirectoryArchive(r.Context(), zw, root, root.Name()); err != nil {
		_ = zw.Close()
		return
	}
	_ = zw.Close()
}

func (s *Server) writeDirectoryArchive(ctx context.Context, zw *zip.Writer, node *share.Node, archivePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if node.Kind() == share.NodeDirectory {
		if err := s.session.Directory().VerifyDirectory(node); err != nil {
			return err
		}
		header := &zip.FileHeader{Name: archivePath + "/", Method: zip.Store}
		header.SetModTime(node.ModTime())
		if _, err := zw.CreateHeader(header); err != nil {
			return err
		}
		for _, child := range node.Children() {
			if err := s.writeDirectoryArchive(ctx, zw, child, path.Join(archivePath, child.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	file, err := s.session.Directory().OpenFile(node)
	if err != nil {
		return err
	}
	defer file.Close()
	header := &zip.FileHeader{Name: archivePath, Method: zip.Deflate}
	header.SetModTime(file.ModTime().Truncate(2 * time.Second))
	entry, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	return copyWithContext(ctx, entry, file.Reader())
}
