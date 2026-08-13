package receive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxFileSize int64 = 1 << 30

var (
	ErrInvalidFilename = errors.New("invalid received filename")
	ErrFileTooLarge    = errors.New("received file exceeds 1 GiB limit")
)

type Store struct {
	dir         string
	maxFileSize int64
}

type Result struct {
	Name string
	Size int64
}

func OpenStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("receive directory is empty")
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve receive directory: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return nil, fmt.Errorf("create receive directory: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("stat receive directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("receive destination is not a directory: %s", absDir)
	}

	return &Store{
		dir:         absDir,
		maxFileSize: MaxFileSize,
	}, nil
}

func (s *Store) Save(ctx context.Context, name string, src io.Reader) (result Result, saveErr error) {
	if err := validateFilename(name); err != nil {
		return Result{}, err
	}

	temporary, err := os.CreateTemp(s.dir, ".qshare-upload-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary upload file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			saveErr = errors.Join(saveErr, fmt.Errorf("remove temporary upload file: %w", err))
		}
	}()

	written, err := io.Copy(temporary, io.LimitReader(&contextReader{ctx: ctx, reader: src}, s.maxFileSize+1))
	if err != nil {
		_ = temporary.Close()
		return Result{}, fmt.Errorf("write received file: %w", err)
	}
	if written > s.maxFileSize {
		_ = temporary.Close()
		return Result{}, ErrFileTooLarge
	}
	if err := temporary.Close(); err != nil {
		return Result{}, fmt.Errorf("close temporary upload file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("write received file: %w", err)
	}

	finalName, err := s.publish(temporaryPath, name)
	if err != nil {
		return Result{}, err
	}

	return Result{Name: finalName, Size: written}, nil
}

func (s *Store) publish(temporaryPath, requestedName string) (string, error) {
	for sequence := 0; ; sequence++ {
		candidate := collisionName(requestedName, sequence)
		finalPath := filepath.Join(s.dir, candidate)
		if err := os.Link(temporaryPath, finalPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("publish received file: %w", err)
		}
		return candidate, nil
	}
}

func validateFilename(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, 0) ||
		strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: %q", ErrInvalidFilename, name)
	}
	return nil
}

func collisionName(name string, sequence int) string {
	if sequence == 0 {
		return name
	}

	extension := filepath.Ext(name)
	if extension == name {
		extension = ""
	}
	stem := strings.TrimSuffix(name, extension)
	return fmt.Sprintf("%s (%d)%s", stem, sequence, extension)
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
