package cli

import (
	"os"

	"golang.org/x/term"
)

func isTerminal(file *os.File) bool {
	return term.IsTerminal(int(file.Fd()))
}
