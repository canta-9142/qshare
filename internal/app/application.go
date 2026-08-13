package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"time"

	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/qr"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

const (
	defaultServerPort      = "55544"
	expirationDrainTimeout = 30 * time.Second
)

type shutdownServer interface {
	Shutdown(context.Context) error
	Close() error
}

type sessionServer interface {
	shutdownServer
	Start(string) (net.Addr, error)
	Done() <-chan error
}

type Application struct {
	stderr           io.Writer
	advertiseAddress func() (netip.Addr, error)
	newServer        func(*session.Session) sessionServer
	renderQR         func(io.Writer, string) error
}

type Dependencies struct {
	Stderr io.Writer
}

func New(deps Dependencies) *Application {
	return &Application{
		stderr:           deps.Stderr,
		advertiseAddress: network.AdvertiseAddress,
		newServer:        func(s *session.Session) sessionServer { return server.New(s) },
		renderQR:         qr.Render,
	}
}

func (a *Application) Run(ctx context.Context, req Request) (runErr error) {
	if req.Operation == OperationReceive {
		return errors.New("receive mode is not implemented")
	}

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
	sess, err := session.New(resource, req.Lifetime)
	if err != nil {
		return err
	}

	// start server
	srv := a.newServer(sess)
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
