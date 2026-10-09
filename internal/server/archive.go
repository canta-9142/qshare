package server

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

func (s *fileHandler) archive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="qshare.zip"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	zw := zip.NewWriter(w)
	used := make(map[string]struct{})
	for _, resource := range s.files.Resources() {
		if err := r.Context().Err(); err != nil {
			panic(http.ErrAbortHandler)
		}
		header := &zip.FileHeader{Name: uniqueArchiveName(resource.Name(), used), Method: zip.Deflate}
		header.SetModTime(resource.File().ModTime())
		entry, err := zw.CreateHeader(header)
		if err != nil {
			panic(http.ErrAbortHandler)
		}
		if err := copyWithContext(r.Context(), entry, resource.File().Reader()); err != nil {
			panic(http.ErrAbortHandler)
		}
	}
	if err := zw.Close(); err != nil {
		panic(http.ErrAbortHandler)
	}
}

func uniqueArchiveName(name string, used map[string]struct{}) string {
	name = safeArchiveName(name)
	if _, exists := used[strings.ToLower(name)]; !exists {
		used[strings.ToLower(name)] = struct{}{}
		return name
	}
	ext := path.Ext(name)
	base := name[:len(name)-len(ext)]
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if _, exists := used[strings.ToLower(candidate)]; !exists {
			used[strings.ToLower(candidate)] = struct{}{}
			return candidate
		}
	}
}

var archiveNameReplacer = strings.NewReplacer("/", "_", "\\", "_", ":", "_", "\x00", "_")

// safeArchiveName sanitizes one ZIP path component independently of the host OS.
func safeArchiveName(name string) string {
	name = strings.TrimRight(archiveNameReplacer.Replace(name), " .")
	if name == "" {
		return "_"
	}
	// Windows device names remain reserved even when followed by an extension.
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return "_" + name
	}
	if strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT") {
		switch stem[3:] {
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return "_" + name
		}
	}
	return name
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, &contextReader{ctx: ctx, reader: src})
	return err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
