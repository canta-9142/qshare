package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
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
	port, err := a.startLANServer(ctx, endpoint, &run)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	accessURLValue := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(endpoint.Address.String(), strconv.Itoa(int(port))),
		Path:   "/s/" + run.session.Token().String(),
	}
	accessURL := accessURLValue.String()
	fmt.Fprintf(a.stderr, "\nQshare\n\n%s\n\n", run.heading)
	if err := a.renderQR(a.stderr, accessURL); err != nil {
		return fmt.Errorf("failed to render QR code: %w", err)
	}
	fmt.Fprintf(a.stderr, "\n%s\n\nThis URL expires after %s.\n\n", accessURL, req.Lifetime)

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

// prepareSession records each acquired resource before the next fallible step.
// All modes return through Run's common cleanup, including preparation failures.
func (a *Application) prepareSession(req Request, run *sessionRun) (err error) {
	switch req.Operation {
	case OperationSendFile:
		run.files, err = a.openCollection(req.Paths)
		if err != nil {
			return err
		}
		run.session, err = session.New(req.Lifetime)
		if err != nil {
			return err
		}
		run.server = server.NewHTTPServer(server.NewSendFile(run.session, run.files))
		run.heading = fmt.Sprintf("Sharing  %d file(s)", len(run.files.Resources()))

	case OperationSendDirectory:
		if len(req.Paths) != 1 {
			return fmt.Errorf("directory send requires exactly one path")
		}
		run.directory, err = a.openDirectory(req.Paths[0])
		if err != nil {
			return err
		}
		run.session, err = session.New(req.Lifetime)
		if err != nil {
			return err
		}
		run.server = server.NewHTTPServer(server.NewSendDirectory(run.session, run.directory))
		run.heading = fmt.Sprintf("Sharing directory  %s", run.directory.Root().Name())

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
