package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func (a *Application) Run(ctx context.Context, req Request) error {
	switch req.Operation {
	case OperationSendFile:
		return a.runSendFile(ctx, req)

	case OperationSendText:
		return a.runSendText(ctx, req)

	case OperationReceive:
		return a.runReceive(ctx, req)

	default:
		return fmt.Errorf("unsupported operation: %d", req.Operation)
	}
}

func (a *Application) runSendFile(ctx context.Context, req Request) (runErr error) {
	resource, err := share.Open(req.Path)
	if err != nil {
		return err
	}

	defer func() {
		if err := resource.Close(); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("failed to close resource: %w", err),
			)
		}
	}()

	// determine advertise address
	advertiseAddr, err := a.advertiseAddress()
	if err != nil {
		return fmt.Errorf("failed to determine LAN advertise address: %w", err)
	}

	// create session
	sess, err := session.NewSendFile(resource, req.Lifetime)
	if err != nil {
		return err
	}

	// start server
	srv := a.newSendServer(sess)
	bindAddr := net.JoinHostPort(advertiseAddr.String(), defaultServerPort)
	listenAddr, err := srv.Start(bindAddr)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// construct URL
	_, port, err := net.SplitHostPort(listenAddr.String())
	if err != nil {
		closeErr := srv.Close()
		return errors.Join(
			fmt.Errorf("failed to parse listen address: %w", err),
			closeErr,
		)
	}
	downloadURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(advertiseAddr.String(), port),
		Path:   "/s/" + sess.Token().String(),
	}

	payload := downloadURL.String()

	fmt.Fprintf(a.stderr, "\nQshare\n\n")
	fmt.Fprintf(a.stderr, "Sharing  %s\n\n", resource.Name())

	if err := a.renderQR(a.stderr, payload); err != nil {
		return errors.Join(
			fmt.Errorf("failed to render QR code: %w", err),
			srv.Close(),
		)
	}

	fmt.Fprintf(a.stderr, "\n%s\n\n", payload)

	fmt.Fprintf(a.stderr, "This URL expires after %s.\n\n", req.Lifetime.String())

	return a.runSession(ctx, sess, srv)
}

func (a *Application) runSendText(ctx context.Context, req Request) error {
	sess, err := session.NewSendText(req.Text, req.Lifetime)
	if err != nil {
		return err
	}

	advertiseAddr, err := a.advertiseAddress()
	if err != nil {
		return fmt.Errorf("failed to determine LAN advertise address: %w", err)
	}

	srv := a.newTextServer(sess)
	bindAddr := net.JoinHostPort(advertiseAddr.String(), defaultServerPort)
	listenAddr, err := srv.Start(bindAddr)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	_, port, err := net.SplitHostPort(listenAddr.String())
	if err != nil {
		return errors.Join(
			fmt.Errorf("failed to parse listen address: %w", err),
			srv.Close(),
		)
	}

	accessURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(advertiseAddr.String(), port),
		Path:   "/s/" + sess.Token().String(),
	}

	fmt.Fprintf(a.stderr, "\nQshare\n\n")
	fmt.Fprintf(a.stderr, "Sharing text\n\n")

	if err := a.renderQR(a.stderr, accessURL.String()); err != nil {
		return errors.Join(
			fmt.Errorf("failed to render QR code: %w", err),
			srv.Close(),
		)
	}

	fmt.Fprintf(a.stderr, "\n%s\n\n", accessURL.String())
	fmt.Fprintf(a.stderr, "This URL expires after %s.\n\n", req.Lifetime)

	return a.runSession(ctx, sess, srv)
}

func (a *Application) runReceive(ctx context.Context, req Request) error {
	store, err := a.openReceiveStore(req.ReceiveDir)
	if err != nil {
		return fmt.Errorf("open receive store: %w", err)
	}

	sess, err := session.NewReceive(req.Lifetime)
	if err != nil {
		return err
	}

	var textSink receive.TextSink = receive.NewWriterTextSink(a.stdout)
	if req.Clipboard != "" {
		textSink, err = a.newClipboardSink(req.Clipboard)
		if err != nil {
			return fmt.Errorf("configure clipboard backend: %w", err)
		}
	}

	textProcessor := receive.NewTextProcessor(
		textSink,
		receive.TextQueueCapacity,
	)
	defer textProcessor.Close()

	advertiseAddr, err := a.advertiseAddress()
	if err != nil {
		return fmt.Errorf("failed to determine LAN advertise address: %w", err)
	}

	srv := a.newReceiveServer(sess, store, textProcessor)
	bindAddr := net.JoinHostPort(advertiseAddr.String(), defaultServerPort)
	listenAddr, err := srv.Start(bindAddr)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	_, port, err := net.SplitHostPort(listenAddr.String())
	if err != nil {
		return errors.Join(
			fmt.Errorf("failed to parse listen address: %w", err),
			srv.Close(),
		)
	}

	accessURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(advertiseAddr.String(), port),
		Path:   "/s/" + sess.Token().String(),
	}

	fmt.Fprintf(a.stderr, "\nQshare\n\n")
	fmt.Fprintf(a.stderr, "Receiving into %s\n\n", req.ReceiveDir)

	if err := a.renderQR(a.stderr, accessURL.String()); err != nil {
		return errors.Join(
			fmt.Errorf("failed to render QR code: %w", err),
			srv.Close(),
		)
	}

	fmt.Fprintf(a.stderr, "\n%s\n\n", accessURL.String())
	fmt.Fprintf(a.stderr, "This URL expires after %s.\n\n", req.Lifetime)

	return a.runSession(ctx, sess, srv)
}

func (a *Application) runSession(ctx context.Context, sess *session.Session, srv sessionServer) error {
	timer := time.NewTimer(time.Until(sess.ExpiresAt()))
	defer timer.Stop()

	select {
	case <-timer.C:
		// Expiration
		if err := shutdownExpiredServer(srv, expirationDrainTimeout); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}
		return nil

	case <-ctx.Done():
		// SIGINT/SIGTERM
		closeErr := srv.Close()
		return errors.Join(
			context.Cause(ctx),
			closeErr,
		)

	case err := <-srv.Done():
		// Server error
		if closeErr := srv.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		if err != nil {
			return fmt.Errorf("HTTP server error: %w", err)
		}
		return nil
	}
}

func shutdownExpiredServer(srv shutdownServer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := srv.Shutdown(ctx)
	if err == nil {
		return nil
	}

	closeErr := srv.Close()
	if errors.Is(err, context.DeadlineExceeded) {
		if closeErr != nil {
			return fmt.Errorf("force close server after drain timeout: %w", closeErr)
		}
		return nil
	}

	return errors.Join(err, closeErr)
}
