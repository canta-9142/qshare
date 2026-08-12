package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

type terminationSignal struct {
	signal os.Signal
}

func (e *terminationSignal) Error() string {
	return fmt.Sprintf("received signal: %s", e.signal)
}

func signalContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case received := <-signals:
			// Restore the default behavior after the first signal.
			// A second Ctrl+C can then terminate the process immediately.
			signal.Stop(signals)
			cancel(&terminationSignal{signal: received})

		case <-ctx.Done():
		}
	}()

	stop := func() {
		signal.Stop(signals)
		cancel(nil)
	}

	return ctx, stop
}
