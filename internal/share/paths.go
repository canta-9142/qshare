package share

import (
	"errors"
	"fmt"
	"os"
)

// ErrInvalidSelection identifies an unsupported number or combination of paths.
// Filesystem and resource validation failures do not belong to this category.
var ErrInvalidSelection = errors.New("invalid shared path selection")

// OpenPaths validates and opens either regular files or a single directory.
// On success, exactly one resource is non-nil and the caller owns its cleanup.
// On failure, neither resource is returned and any partial resources are closed.
func OpenPaths(paths []string) (*Collection, *Directory, error) {
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("%w: at least one shared path is required", ErrInvalidSelection)
	}
	if len(paths) > MaxFiles {
		return nil, nil, fmt.Errorf("%w: too many files: got %d, maximum is %d", ErrInvalidSelection, len(paths), MaxFiles)
	}

	// Check combinations before opening anything. A missing or unreadable path
	// must not hide a directory combined with another path, in either order.
	// The open functions still perform their own filesystem safety checks.
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err == nil && info.IsDir() {
			if len(paths) != 1 {
				return nil, nil, fmt.Errorf("%w: a directory cannot be combined with another path", ErrInvalidSelection)
			}
			directory, err := OpenDirectory(path)
			return nil, directory, err
		}
	}
	files, err := OpenCollection(paths)
	return files, nil, err
}
