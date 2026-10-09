package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func (a *Application) Run(ctx context.Context, req Request) (runErr error) {
	var run sessionRun
	end := sessionEnded
	defer func() { runErr = run.finish(ctx, end, runErr) }()

	if err := a.prepareSession(req, &run); err != nil {
		return err
	}
	endpoint, err := a.advertiseEndpoint()
	if err != nil {
		return fmt.Errorf("failed to determine LAN advertise address: %w", err)
	}
	startupCtx, cancelStartup := context.WithDeadline(ctx, run.session.ExpiresAt())
	defer cancelStartup()
	port, err := a.prepareLANServer(startupCtx, endpoint, &run, req.Port)
	if err == nil {
		err = startupCtx.Err()
	}
	if err != nil {
		if onlyStartupCancellation(err, startupCtx.Err()) {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			end = sessionExpired
			return nil
		}
		if cause := context.Cause(ctx); cause != nil && errors.Is(err, startupCtx.Err()) {
			err = errors.Join(cause, err)
		}
		return fmt.Errorf("failed to start server: %w", err)
	}

	accessURLValue := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(endpoint.Address.String(), strconv.Itoa(int(port))),
		Path:   "/s/" + run.session.Token().String(),
	}
	accessURL := accessURLValue.String()
	var display bytes.Buffer
	fmt.Fprintf(&display, "\nQshare\n\n%s\n\n", run.heading)
	if err := a.renderQR(&display, accessURL); err != nil {
		return fmt.Errorf("failed to render QR code: %w", err)
	}
	fmt.Fprintf(&display, "\n%s\n\nThis URL expires at %s.\n\n", accessURL, run.session.ExpiresAt().Format(time.RFC3339))
	// Include QR generation in startup, and check validity before publishing it.
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if !time.Now().Before(run.session.ExpiresAt()) {
		end = sessionExpired
		return nil
	}
	run.serve()
	if _, err := display.WriteTo(a.stderr); err != nil {
		return fmt.Errorf("failed to display session: %w", err)
	}

	var shutdownRequested <-chan struct{}
	if a.startShutdownListener != nil {
		var restore func() error
		shutdownRequested, restore, err = a.startShutdownListener()
		if err != nil {
			return fmt.Errorf("configure quit key: %w", err)
		}
		run.watchTerminal(ctx, restore)
		if shutdownRequested != nil {
			fmt.Fprint(a.stderr, "Press q to quit.\n\n")
		}
	}
	end, runErr = run.wait(ctx, shutdownRequested)
	return runErr
}

// onlyStartupCancellation must not discard other failures joined with cancellation.
func onlyStartupCancellation(err, cancellation error) bool {
	if err == nil || cancellation == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		for _, child := range children {
			if !onlyStartupCancellation(child, cancellation) {
				return false
			}
		}
		return len(children) > 0
	}
	if wrapped := errors.Unwrap(err); wrapped != nil {
		return onlyStartupCancellation(wrapped, cancellation)
	}
	return err == cancellation
}

// prepareSession records each acquired resource before the next fallible step.
// All modes return through Run's common cleanup, including preparation failures.
func (a *Application) prepareSession(req Request, run *sessionRun) (err error) {
	switch req.Operation {
	case OperationSendPaths:
		run.files, run.directory, err = a.openPaths(req.Paths)
		if err != nil {
			if errors.Is(err, share.ErrInvalidSelection) {
				return invalidRequest(err)
			}
			return err
		}
		run.session, err = session.New(req.Lifetime)
		if err != nil {
			return err
		}
		if run.directory != nil {
			run.server = server.NewHTTPServer(server.NewSendDirectory(run.session, run.directory))
			run.heading = fmt.Sprintf("Sharing directory  %s", run.directory.Root().Name())
		} else {
			run.server = server.NewHTTPServer(server.NewSendFile(run.session, run.files))
			run.heading = fmt.Sprintf("Sharing  %d file(s)", len(run.files.Resources()))
		}

	case OperationSendText:
		run.session, err = session.New(req.Lifetime)
		if err != nil {
			return err
		}
		run.server = server.NewHTTPServer(server.NewSendText(run.session, req.Text))
		run.heading = "Sharing text"

	case OperationReceive:
		sink, err := a.receiveTextSink(req.Clipboard)
		if err != nil {
			return err
		}
		store, err := a.openReceiveStore(req.ReceiveDir)
		if err != nil {
			return fmt.Errorf("open receive store: %w", err)
		}
		run.session, err = session.New(req.Lifetime)
		if err != nil {
			return err
		}
		run.textProcessor = receive.NewTextProcessor(sink, receive.TextQueueCapacity)
		run.server = server.NewHTTPServer(server.NewReceive(run.session, store, run.textProcessor))
		run.heading = "Receiving into " + req.ReceiveDir

	default:
		return fmt.Errorf("unsupported operation: %d", req.Operation)
	}
	return nil
}

func (a *Application) receiveTextSink(backend string) (receive.TextSink, error) {
	if backend == "" {
		return receive.NewWriterTextSink(a.stdout), nil
	}
	sink, err := a.newClipboardSink(backend)
	if err != nil {
		if backend == "auto" && errors.Is(err, clipboard.ErrBackendNotFound) {
			fmt.Fprintln(a.stderr, "Clipboard backend not found; received text will be written to stdout.")
			return receive.NewWriterTextSink(a.stdout), nil
		}
		if errors.Is(err, ErrInvalidRequest) {
			return nil, err
		}
		return nil, fmt.Errorf("configure clipboard backend: %w", err)
	}
	return sink, nil
}
