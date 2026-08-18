package share

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type File struct {
	file    *os.File
	name    string
	size    int64
	modTime time.Time
}

func Open(path string) (*File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lstat shared file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("shared file is not a regular file: %s", path)
	}

	f, err := openFileNoFollow(path)
	if err != nil {
		return nil, err
	}

	info, err = f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat shared file: %w", err)
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("shared file is not a regular file: %s", path)
	}

	return &File{
		file:    f,
		name:    filepath.Base(path),
		size:    info.Size(),
		modTime: info.ModTime(),
	}, nil
}

func (f *File) Name() string {
	return f.name
}

func (f *File) Size() int64 {
	return f.size
}

func (f *File) ModTime() time.Time {
	return f.modTime
}

func (f *File) Reader() io.ReadSeeker {
	return io.NewSectionReader(f.file, 0, f.size)
}

func (f *File) Close() error {
	return f.file.Close()
}
