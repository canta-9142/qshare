package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/canta-9142/qshare/internal/app"
)

func RunWithStdin(argv []string, version string, stdin *os.File, stdout io.Writer, stderr io.Writer) int {
	stdinIsTerminal := isTerminal(stdin)
	var startQuitListener quitListenerStarter
	if stdinIsTerminal {
		startQuitListener = func() (terminalQuitListener, error) {
			return startTerminalQuitListener(stdin)
		}
	}
	return runWithInputAndQuitListener(argv, version, stdin, stdinIsTerminal, stdout, stderr, startQuitListener)
}

type terminalQuitListener interface {
	Quit() <-chan struct{}
	Close() error
}

type quitListenerStarter func() (terminalQuitListener, error)

func runWithInputAndQuitListener(
	argv []string,
	version string,
	stdin io.Reader,
	stdinIsTerminal bool,
	stdout io.Writer,
	stderr io.Writer,
	startQuitListener quitListenerStarter,
) int {
	result, err := parseWithInput(argv, stdinInput{
		reader:   stdin,
		terminal: stdinIsTerminal,
	}, version, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "qshare: %v\n", err)
		return 2
	}

	if result.Exit {
		return result.Code
	}

	ctx, stopSignals := signalContext(context.Background())
	defer stopSignals()

	var startShutdownListener func() (<-chan struct{}, func() error, error)
	if startQuitListener != nil {
		startShutdownListener = func() (<-chan struct{}, func() error, error) {
			listener, err := startQuitListener()
			if err != nil {
				return nil, nil, err
			}
			return listener.Quit(), listener.Close, nil
		}
	}
	application := app.New(app.Dependencies{
		Stdout:                stdout,
		Stderr:                stderr,
		StartShutdownListener: startShutdownListener,
	})

	err = application.Run(ctx, result.Request)
	if err != nil {
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
