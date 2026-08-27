package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/platform/firewall"
	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
)

type sessionEnd uint8

const (
	sessionEnded sessionEnd = iota
	sessionShutdownRequested
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

	// create session
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
	textProcessorStopped := false
	defer func() {
		if !textProcessorStopped {
			textProcessor.Close()
		}
	}()

	newServer := func(sess *session.Session) sessionServer {
		return a.newReceiveServer(sess, store, textProcessor)
	}
	end, err := a.runPreparedSession(ctx, sess, newServer, "Receiving into "+req.ReceiveDir, req.Lifetime)
	if err != nil || end != sessionShutdownRequested {
		return err
	}

	err = shutdownTextProcessor(ctx, textProcessor)
	textProcessorStopped = true
	return err
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
	a.printQuitHint()

	return a.runSession(ctx, sess, srv)
}

func (a *Application) printQuitHint() {
	if a.shutdownRequested != nil {
		fmt.Fprint(a.stderr, "Press q to quit.\n\n")
	}
}

// startLANServer binds an available random port and opens its temporary firewall rule.
func (a *Application) startLANServer(
	ctx context.Context,
	endpoint network.Endpoint,
	sess *session.Session,
	srv sessionServer,
) (sessionServer, string, error) {
	initialPort, err := a.selectServerPort()
	if err != nil {
		return nil, "", err
	}
	if initialPort < minimumServerPort || initialPort >= minimumServerPort+serverPortCount {
		return nil, "", fmt.Errorf("selected server port %d is outside the configured range", initialPort)
	}

	var listenAddr net.Addr
	// A random starting point keeps normal selection unpredictable. Advancing
	// within the range guarantees that collision retries do not repeat a port.
	for attempt := 0; attempt < serverPortAttempts; attempt++ {
		port := minimumServerPort + (int(initialPort)-minimumServerPort+attempt)%serverPortCount
		bindAddr := net.JoinHostPort(endpoint.Address.String(), strconv.FormatUint(uint64(port), 10))
		listenAddr, err = srv.Start(bindAddr)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, "", err
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf(
			"failed to find an available port in %d-%d after %d attempts: %w",
			minimumServerPort,
			minimumServerPort+serverPortCount-1,
			serverPortAttempts,
			err,
		)
	}

	_, portText, err := net.SplitHostPort(listenAddr.String())
	if err != nil {
		return nil, "", errors.Join(
			fmt.Errorf("failed to parse listen address: %w", err),
			srv.Close(),
		)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		if err == nil {
			err = errors.New("port must not be zero")
		}
		return nil, "", errors.Join(
			fmt.Errorf("failed to parse listen port %q: %w", portText, err),
			srv.Close(),
		)
	}

	lease, err := a.openFirewall(ctx, firewall.Rule{
		Interface:   endpoint.Interface,
		Source:      endpoint.Prefix,
		Destination: endpoint.Address,
		Port:        uint16(port),
		Timeout:     time.Until(sess.ExpiresAt()) + expirationDrainTimeout + firewallTimeoutSlack,
	})
	if err != nil {
		return nil, "", errors.Join(
			fmt.Errorf("failed to configure firewall: %w", err),
			srv.Close(),
		)
	}

	return &firewalledSessionServer{
		sessionServer: srv,
		lease:         lease,
	}, portText, nil
}

// firewalledSessionServer couples HTTP shutdown with firewall cleanup.
type firewalledSessionServer struct {
	sessionServer
	lease firewallLease
}

// Shutdown gracefully stops HTTP traffic and removes the firewall rule.
func (s *firewalledSessionServer) Shutdown(ctx context.Context) error {
	return errors.Join(s.sessionServer.Shutdown(ctx), s.closeFirewall())
}

// Close immediately stops HTTP traffic and removes the firewall rule.
func (s *firewalledSessionServer) Close() error {
	return errors.Join(s.sessionServer.Close(), s.closeFirewall())
}

// closeFirewall bounds cleanup independently from the session context.
func (s *firewalledSessionServer) closeFirewall() error {
	ctx, cancel := context.WithTimeout(context.Background(), firewallCleanupTimeout)
	defer cancel()
	if err := s.lease.Close(ctx); err != nil {
		return fmt.Errorf("remove temporary firewall rule: %w", err)
	}
	return nil
}

func (a *Application) runSession(ctx context.Context, sess *session.Session, srv sessionServer) (sessionEnd, error) {
	timer := time.NewTimer(time.Until(sess.ExpiresAt()))
	defer timer.Stop()

	select {
	case <-timer.C:
		// Expiration
		if err := shutdownExpiredServer(srv, expirationDrainTimeout); err != nil {
			return sessionEnded, fmt.Errorf("failed to shutdown server: %w", err)
		}
		return sessionEnded, nil

	case <-a.shutdownRequested:
		// Interactive normal shutdown
		if err := shutdownRequestedServer(ctx, srv, expirationDrainTimeout); err != nil {
			return sessionShutdownRequested, fmt.Errorf("failed to shutdown server: %w", err)
		}
		return sessionShutdownRequested, nil

	case <-ctx.Done():
		// SIGINT/SIGTERM
		closeErr := srv.Close()
		return sessionEnded, errors.Join(
			context.Cause(ctx),
			closeErr,
		)

	case err := <-srv.Done():
		// Server error
		if closeErr := srv.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		if err != nil {
			return sessionEnded, fmt.Errorf("HTTP server error: %w", err)
		}
		return sessionEnded, nil
	}
}

func shutdownExpiredServer(srv shutdownServer, timeout time.Duration) error {
	return shutdownSessionServer(context.Background(), srv, timeout)
}

func shutdownRequestedServer(ctx context.Context, srv shutdownServer, timeout time.Duration) error {
	return shutdownSessionServer(ctx, srv, timeout)
}

func shutdownSessionServer(parent context.Context, srv shutdownServer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	err := srv.Shutdown(ctx)
	if err == nil {
		return nil
	}

	closeErr := srv.Close()
	if cause := context.Cause(parent); cause != nil {
		return errors.Join(cause, closeErr)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if closeErr != nil {
			return fmt.Errorf("force close server after drain timeout: %w", closeErr)
		}
		return nil
	}

	return errors.Join(err, closeErr)
}

func shutdownTextProcessor(ctx context.Context, processor *receive.TextProcessor) error {
	done := make(chan struct{})
	go func() {
		processor.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		processor.Close()
		<-done
		return context.Cause(ctx)
	}
}
