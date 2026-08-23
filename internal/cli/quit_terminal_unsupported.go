//go:build !linux

package cli

import (
	"errors"
	"os"
)

func startTerminalQuitListener(*os.File) (terminalQuitListener, error) {
	return nil, errors.New("single-key quit is unsupported on this platform")
}
