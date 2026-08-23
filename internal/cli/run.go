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
	stdinIsTerminal := isTerminal(stdin)
	var startQuitListener quitListenerStarter
	if stdinIsTerminal {
		startQuitListener = func() (terminalQuitListener, error) {
			return startTerminalQuitListener(stdin)
		}
	}
	return runWithInputAndQuitListener(argv, stdin, stdinIsTerminal, stdout, stderr, startQuitListener)
}

func runWithInput(argv []string, stdin io.Reader, stdinIsTerminal bool, stdout io.Writer, stderr io.Writer) int {
	return runWithInputAndQuitListener(argv, stdin, stdinIsTerminal, stdout, stderr, nil)
}

type terminalQuitListener interface {
	Quit() <-chan struct{}
	Close() error
}

type quitListenerStarter func() (terminalQuitListener, error)

func runWithInputAndQuitListener(
	argv []string,
	stdin io.Reader,
	stdinIsTerminal bool,
	stdout io.Writer,
	stderr io.Writer,
	startQuitListener quitListenerStarter,
) int {
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

	var quitListener terminalQuitListener
	if startQuitListener != nil {
		quitListener, err = startQuitListener()
		if err != nil {
			fmt.Fprintf(stderr, "qshare: configure quit key: %v\n", err)
			return 1
		}
	}

	listenerWatchDone := make(chan struct{})
	var listenerWatchExited chan struct{}
	if quitListener != nil {
		listenerWatchExited = make(chan struct{})
		go func() {
			defer close(listenerWatchExited)
			select {
			case <-ctx.Done():
				_ = quitListener.Close()
			case <-listenerWatchDone:
			}
		}()
	}

	var shutdownRequested <-chan struct{}
	if quitListener != nil {
		shutdownRequested = quitListener.Quit()
	}
	application := app.New(app.Dependencies{
		Stdout:            stdout,
		Stderr:            stderr,
		ShutdownRequested: shutdownRequested,
	})

	err = application.Run(ctx, result.Request)
	close(listenerWatchDone)
	if quitListener != nil {
		err = errors.Join(err, quitListener.Close())
		<-listenerWatchExited
	}
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
