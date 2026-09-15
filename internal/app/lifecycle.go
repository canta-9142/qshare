package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

type sessionEnd uint8

const (
	sessionEnded sessionEnd = iota // Startup failure, cancellation, or server exit.
	sessionExpired
	sessionShutdownRequested
)

// sessionRun owns the resources of one Run, including partial startup.
// The listener is owned even before Serve starts; the terminal supplies restore.
type sessionRun struct {
	session       *session.Session
	server        *http.Server
	listener      net.Listener
	serverDone    chan error
	heading       string
	lease         firewallLease
	files         *share.Collection
	directory     *share.Directory
	textProcessor *receive.TextProcessor

	restoreRequested chan struct{}
	restoreResult    chan error
}

func (r *sessionRun) serve() {
	r.serverDone = make(chan error, 1)
	go func() {
		err := r.server.Serve(r.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		r.serverDone <- err
		close(r.serverDone)
	}()
}

func (r *sessionRun) watchTerminal(ctx context.Context, restore func() error) {
	if restore == nil {
		return
	}
	// One goroutine restores the terminal on cancellation or normal cleanup.
	// Its buffered result lets restoration finish even while other cleanup waits.
	r.restoreRequested = make(chan struct{})
	r.restoreResult = make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
		case <-r.restoreRequested:
		}
		r.restoreResult <- restore()
	}()
}

func (r *sessionRun) wait(ctx context.Context, shutdownRequested <-chan struct{}) (sessionEnd, error) {
	timer := time.NewTimer(time.Until(r.session.ExpiresAt()))
	defer timer.Stop()

	select {
	case <-timer.C:
		return sessionExpired, nil
	case <-shutdownRequested:
		return sessionShutdownRequested, nil
	case <-ctx.Done():
		return sessionEnded, context.Cause(ctx)
	case err := <-r.serverDone:
		if err != nil {
			return sessionEnded, fmt.Errorf("HTTP server error: %w", err)
		}
		return sessionEnded, nil
	}
}

// finish is the common exit path. Stop HTTP before removing its firewall rule,
// then finish text processing, release shared files, and restore the terminal.
// Only cancellation restores the terminal independently of that sequence.
func (r *sessionRun) finish(ctx context.Context, end sessionEnd, runErr error) error {
	if r.server != nil {
		var err error
		switch end {
		case sessionExpired:
			// An expiration drain is intentionally independent of signals.
			err = shutdownSessionServer(context.Background(), r.server, expirationDrainTimeout)
		case sessionShutdownRequested:
			err = shutdownSessionServer(ctx, r.server, expirationDrainTimeout)
		default:
			err = r.server.Close()
		}
		if err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("failed to shutdown server: %w", err))
		}
	}
	// Serve may not have started (for example, firewall setup failed), so the
	// application must also close the listener. HTTP may already have closed it.
	if r.listener != nil {
		if err := r.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			runErr = errors.Join(runErr, fmt.Errorf("close listener: %w", err))
		}
	}
	if r.serverDone != nil {
		if err := <-r.serverDone; err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("HTTP server error: %w", err))
		}
	}
	if r.lease != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), firewallCleanupTimeout)
		err := r.lease.Close(cleanupCtx)
		cancel()
		if err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("remove temporary firewall rule: %w", err))
		}
	}
	if r.textProcessor != nil {
		// Preserve the q-only drain, and skip it if HTTP or firewall cleanup failed.
		if end == sessionShutdownRequested && runErr == nil {
			runErr = r.textProcessor.Shutdown(ctx)
		}
		r.textProcessor.Close()
	}
	if r.files != nil {
		if err := r.files.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("failed to close resource: %w", err))
		}
	}
	if r.directory != nil {
		if err := r.directory.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("failed to close directory: %w", err))
		}
	}
	if r.restoreRequested != nil {
		close(r.restoreRequested)
		runErr = errors.Join(runErr, <-r.restoreResult)
	}
	return runErr
}

func shutdownSessionServer(parent context.Context, srv *http.Server, timeout time.Duration) error {
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
