package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/canta-9142/qshare/internal/app"
)

func Run(argv []string, stdout io.Writer, stderr io.Writer) int {
	return runWithInput(argv, nil, true, stdout, stderr)
}

func RunWithStdin(argv []string, stdin *os.File, stdout io.Writer, stderr io.Writer) int {
	return runWithInput(argv, stdin, isTerminal(stdin), stdout, stderr)
}

func runWithInput(argv []string, stdin io.Reader, stdinIsTerminal bool, stdout io.Writer, stderr io.Writer) int {
	result, err := parseWithInput(argv, stdinInput{
		reader:   stdin,
		terminal: stdinIsTerminal,
	}, stdout, stderr)
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
		Stdout: stdout,
		Stderr: stderr,
	})

	if err := application.Run(ctx, result.Request); err != nil {
		fmt.Fprintf(stderr, "qshare: %v\n", err)
		return exitCodeForError(err)
	}

	return 0
}

func exitCodeForError(err error) int {
	if errors.Is(err, app.ErrInvalidRequest) {
		return 2
	}

	var signalErr *terminationSignal
	if !errors.As(err, &signalErr) {
		return 1
	}
	if !containsOnlyTerminationErrors(err) {
		return 1
	}

	return signalErr.exitCode()
}

func containsOnlyTerminationErrors(err error) bool {
	if err == nil {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !containsOnlyTerminationErrors(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return containsOnlyTerminationErrors(wrapped.Unwrap())
	}
	var signalErr *terminationSignal
	return errors.As(err, &signalErr)
}
