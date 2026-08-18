//go:build linux || darwin

package share

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openDirectoryNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		unix.Close(fd)
		return nil, errors.New("create directory handle")
	}
	return file, nil
}

func openDirectoryEntryNoFollow(dir *os.File, name string) (*os.File, os.FileInfo, bool, error) {
	var observed unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), name, &observed, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, nil, false, fmt.Errorf("stat directory entry %q: %w", name, err)
	}
	observedMode := uint32(observed.Mode) & unix.S_IFMT
	if observedMode != unix.S_IFREG && observedMode != unix.S_IFDIR {
		return nil, nil, false, nil
	}

	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, false, fmt.Errorf("open directory entry %q: %w", name, err)
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		unix.Close(fd)
		return nil, nil, false, errors.New("create directory entry handle")
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, false, fmt.Errorf("stat directory entry %q: %w", name, err)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		file.Close()
		return nil, nil, false, nil
	}
	return file, info, true, nil
}

func reopenDirectoryEntryNoFollow(dir *os.File, name string, directory bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if directory {
		flags |= unix.O_DIRECTORY
	} else {
		flags |= unix.O_NONBLOCK
	}
	fd, err := unix.Openat(int(dir.Fd()), name, flags, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		unix.Close(fd)
		return nil, errors.New("create reopened directory entry handle")
	}
	return file, nil
}
