package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/canta-9142/qshare/internal/app"
)

func Run(argv []string, stdout io.Writer, stderr io.Writer) int {
	result, err := parse(argv, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "qshare: %v\n", err)
		return 2
	}

	if result.Exit {
		return result.Code
	}

	ctx, stopSignals := signalContext(context.Background())
	defer stopSignals()

	application := app.New(app.Dependencies{
		Stderr: stderr,
	})

	if err := application.Run(ctx, result.Request); err != nil {
		fmt.Fprintf(stderr, "qshare: %v\n", err)
		return exitCodeForError(err)
	}

	return 0
}

func exitCodeForError(err error) int {
	var signalErr *terminationSignal
	if !errors.As(err, &signalErr) {
		return 1
	}

	switch signalErr.signal {
	case os.Interrupt:
		return 130
	case syscall.SIGTERM:
		return 143
	default:
		return 1
	}
}
