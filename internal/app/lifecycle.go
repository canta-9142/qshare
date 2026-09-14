package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/canta-9142/qshare/internal/session"
)

type sessionEnd uint8

const (
	sessionEnded sessionEnd = iota
	sessionShutdownRequested
)

func (a *Application) enableInteractiveShutdown(srv sessionServer) (<-chan struct{}, error) {
	if a.startShutdownListener == nil {
		return nil, nil
	}
	shutdownRequested, err := a.startShutdownListener()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("configure quit key: %w", err), srv.Close())
	}
	if shutdownRequested != nil {
		fmt.Fprint(a.stderr, "Press q to quit.\n\n")
	}
	return shutdownRequested, nil
}

func (a *Application) runSession(ctx context.Context, sess *session.Session, srv sessionServer, shutdownRequested <-chan struct{}) (sessionEnd, error) {
	timer := time.NewTimer(time.Until(sess.ExpiresAt()))
	defer timer.Stop()

	select {
	case <-timer.C:
		// Expiration
		if err := shutdownSessionServer(context.Background(), srv, expirationDrainTimeout); err != nil {
			return sessionEnded, fmt.Errorf("failed to shutdown server: %w", err)
		}
		return sessionEnded, nil

	case <-shutdownRequested:
		// Interactive normal shutdown
		if err := shutdownSessionServer(ctx, srv, expirationDrainTimeout); err != nil {
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
