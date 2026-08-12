package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

type Application struct {
	stderr io.Writer
}

type Dependencies struct {
	Stderr io.Writer
}

func New(deps Dependencies) *Application {
	return &Application{
		stderr: deps.Stderr,
	}
}

func (a *Application) Run(ctx context.Context, req Request) (runErr error) {
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
	advertiseAddr, err := network.AdvertiseAddress()
	if err != nil {
		return fmt.Errorf("failed to determine LAN advertise address: %w", err)
	}

	// create session
	sess, err := session.New(resource, req.Lifetime)
	if err != nil {
		return err
	}

	// start server
	srv := server.New(sess)
	bindAddr := net.JoinHostPort(advertiseAddr.String(), "0")
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
		Path:   "/d/" + sess.Token().String(),
	}

	// render QR
	// if err := qr.Render(...

	fmt.Fprintf(
		a.stderr,
		"Sharing %s\n%s\n",
		resource.Name(),
		downloadURL.String(),
	)

	return a.runSession(ctx, sess, srv)
}

func (a *Application) runSession(ctx context.Context, sess *session.Session, srv *server.Server) error {
	timer := time.NewTimer(time.Until(sess.ExpiresAt()))
	defer timer.Stop()

	select {
	case <-timer.C:
		// Expiration
		if err := srv.Shutdown(context.Background()); err != nil {
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
