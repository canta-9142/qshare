package app

import (
	"context"
	"errors"
	"fmt"
	"io"

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

	sess, err := session.New(resource, req.Lifetime)
	if err != nil {
		return err
	}

	srv := server.New(sess)

	// bind
	// construct URL
	// render QR
	// wait until expiry / ctx cancellation
	// graceful shutdown

	return a.runSession(ctx, sess, srv)
}

func (a *Application) runSession(ctx context.Context, sess *session.Session, srv *server.Server) error {
	// Start the server in a separate goroutine
	// Wait for the session to expire or for the context to be canceled
	// Gracefully shut down the server
	return nil
}
