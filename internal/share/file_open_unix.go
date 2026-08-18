//go:build linux || darwin

package share

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openFileNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open shared file without following final symlink: %w", err)
	}

	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		unix.Close(fd)
		return nil, fmt.Errorf("create os.File from file descriptor")
	}

	return f, nil
}
