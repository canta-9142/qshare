package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownExpiredServer(t *testing.T) {
	t.Run("graceful drain", func(t *testing.T) {
		server := &fakeShutdownServer{}

		if err := shutdownExpiredServer(server, time.Second); err != nil {
			t.Fatalf("shutdownExpiredServer() error = %v", err)
		}
		if server.closeCalls != 0 {
			t.Fatalf("Close() calls = %d, want 0", server.closeCalls)
		}
	})

	t.Run("drain timeout forces close and remains successful", func(t *testing.T) {
		server := &fakeShutdownServer{
			shutdown: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}

		if err := shutdownExpiredServer(server, 0); err != nil {
			t.Fatalf("shutdownExpiredServer() error = %v", err)
		}
		if server.closeCalls != 1 {
			t.Fatalf("Close() calls = %d, want 1", server.closeCalls)
		}
	})

	t.Run("force close failure is reported", func(t *testing.T) {
		closeErr := errors.New("close failed")
		server := &fakeShutdownServer{
			shutdown: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			closeErr: closeErr,
		}

		err := shutdownExpiredServer(server, 0)
		if !errors.Is(err, closeErr) {
			t.Fatalf("shutdownExpiredServer() error = %v, want close error", err)
		}
		if server.closeCalls != 1 {
			t.Fatalf("Close() calls = %d, want 1", server.closeCalls)
		}
	})

	t.Run("shutdown failure is reported after close", func(t *testing.T) {
		shutdownErr := errors.New("shutdown failed")
		server := &fakeShutdownServer{
			shutdown: func(context.Context) error {
				return shutdownErr
			},
		}

		err := shutdownExpiredServer(server, time.Second)
		if !errors.Is(err, shutdownErr) {
			t.Fatalf("shutdownExpiredServer() error = %v, want shutdown error", err)
		}
		if server.closeCalls != 1 {
			t.Fatalf("Close() calls = %d, want 1", server.closeCalls)
		}
	})
}

type fakeShutdownServer struct {
	shutdown   func(context.Context) error
	closeErr   error
	closeCalls int
}

func (s *fakeShutdownServer) Shutdown(ctx context.Context) error {
	if s.shutdown == nil {
		return nil
	}
	return s.shutdown(ctx)
}

func (s *fakeShutdownServer) Close() error {
	s.closeCalls++
	return s.closeErr
}
