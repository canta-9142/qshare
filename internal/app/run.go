package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
)

func (a *Application) Run(ctx context.Context, req Request) error {
	switch req.Operation {
	case OperationSendFile:
		return a.runSendFile(ctx, req)
	case OperationSendDirectory:
		return a.runSendDirectory(ctx, req)

	case OperationSendText:
		return a.runSendText(ctx, req)

	case OperationReceive:
		return a.runReceive(ctx, req)

	default:
		return fmt.Errorf("unsupported operation: %d", req.Operation)
	}
}

func (a *Application) runSendDirectory(ctx context.Context, req Request) (runErr error) {
	if len(req.Paths) != 1 {
		return fmt.Errorf("directory send requires exactly one path")
	}
	directory, err := a.openDirectory(req.Paths[0])
	if err != nil {
		return err
	}
	defer func() {
		if err := directory.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("failed to close directory: %w", err))
		}
	}()
	sess, err := session.NewSendDirectory(directory, req.Lifetime)
	if err != nil {
		return err
	}
	_, err = a.runPreparedSession(ctx, sess, a.newDirectoryServer, fmt.Sprintf("Sharing directory  %s", directory.Root().Name()), req.Lifetime)
	return err
}

func (a *Application) runSendFile(ctx context.Context, req Request) (runErr error) {
	resources, err := a.openCollection(req.Paths)
	if err != nil {
		return err
	}

	defer func() {
		if err := resources.Close(); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("failed to close resource: %w", err),
			)
		}
	}()

	sess, err := session.NewSendFiles(resources, req.Lifetime)
	if err != nil {
		return err
	}

	_, err = a.runPreparedSession(ctx, sess, a.newSendServer, fmt.Sprintf("Sharing  %d file(s)", len(resources.Resources())), req.Lifetime)
	return err
}

func (a *Application) runSendText(ctx context.Context, req Request) error {
	sess, err := session.NewSendText(req.Text, req.Lifetime)
	if err != nil {
		return err
	}

	_, err = a.runPreparedSession(ctx, sess, a.newTextServer, "Sharing text", req.Lifetime)
	return err
}

func (a *Application) runReceive(ctx context.Context, req Request) error {
	var textSink receive.TextSink = receive.NewWriterTextSink(a.stdout)
	if req.Clipboard != "" {
		var err error
		textSink, err = a.newClipboardSink(req.Clipboard)
		if err != nil {
			if req.Clipboard == "auto" && errors.Is(err, clipboard.ErrBackendNotFound) {
				fmt.Fprintln(a.stderr, "Clipboard backend not found; received text will be written to stdout.")
				textSink = receive.NewWriterTextSink(a.stdout)
			} else if errors.Is(err, ErrInvalidRequest) {
				return err
			} else {
				return fmt.Errorf("configure clipboard backend: %w", err)
			}
		}
	}

	store, err := a.openReceiveStore(req.ReceiveDir)
	if err != nil {
		return fmt.Errorf("open receive store: %w", err)
	}

	sess, err := session.NewReceive(req.Lifetime)
	if err != nil {
		return err
	}

	textProcessor := receive.NewTextProcessor(
		textSink,
		receive.TextQueueCapacity,
	)
	defer textProcessor.Close()

	newServer := func(sess *session.Session) sessionServer {
		return a.newReceiveServer(sess, store, textProcessor)
	}
	end, err := a.runPreparedSession(ctx, sess, newServer, "Receiving into "+req.ReceiveDir, req.Lifetime)
	if err != nil || end != sessionShutdownRequested {
		return err
	}

	return textProcessor.Shutdown(ctx)
}

func (a *Application) runPreparedSession(
	ctx context.Context,
	sess *session.Session,
	newServer func(*session.Session) sessionServer,
	heading string,
	lifetime time.Duration,
) (sessionEnd, error) {
	endpoint, err := a.advertiseEndpoint()
	if err != nil {
		return sessionEnded, fmt.Errorf("failed to determine LAN advertise address: %w", err)
	}

	srv, port, err := a.startLANServer(ctx, endpoint, sess, newServer(sess))
	if err != nil {
		return sessionEnded, fmt.Errorf("failed to start server: %w", err)
	}

	accessURLValue := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(endpoint.Address.String(), port),
		Path:   "/s/" + sess.Token().String(),
	}
	accessURL := accessURLValue.String()

	fmt.Fprintf(a.stderr, "\nQshare\n\n%s\n\n", heading)
	if err := a.renderQR(a.stderr, accessURL); err != nil {
		return sessionEnded, errors.Join(
			fmt.Errorf("failed to render QR code: %w", err),
			srv.Close(),
		)
	}

	fmt.Fprintf(a.stderr, "\n%s\n\nThis URL expires after %s.\n\n", accessURL, lifetime)
	shutdownRequested, err := a.enableInteractiveShutdown(srv)
	if err != nil {
		return sessionEnded, err
	}

	return a.runSession(ctx, sess, srv, shutdownRequested)
}
