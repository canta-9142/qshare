package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/session"
)

func TestApplicationRunExpiration(t *testing.T) {
	application, fake, stderr, path := newTestApplication(t)
	err := application.Run(context.Background(), Request{Path: path, Lifetime: time.Millisecond})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fake.shutdownCalls != 1 || fake.closeCalls != 0 {
		t.Fatalf("shutdown=%d close=%d", fake.shutdownCalls, fake.closeCalls)
	}
	if got := stderr.String(); !strings.Contains(got, "http://192.0.2.10:55544/s/") || !strings.Contains(got, "This URL expires after 1ms") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestApplicationRunCancellation(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	cause := errors.New("cancelled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{Path: path, Lifetime: time.Hour})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cause", err)
	}
	if fake.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
	}
}

func TestApplicationRunServerFailure(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	serveErr := errors.New("serve failed")
	fake.done <- serveErr
	close(fake.done)
	err := application.Run(context.Background(), Request{Path: path, Lifetime: time.Hour})
	if !errors.Is(err, serveErr) {
		t.Fatalf("Run() error = %v, want server error", err)
	}
	if fake.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
	}
}

func TestApplicationRunClosesServerWhenQRRenderingFails(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	renderErr := errors.New("render failed")
	application.renderQR = func(io.Writer, string) error { return renderErr }
	err := application.Run(context.Background(), Request{Path: path, Lifetime: time.Hour})
	if !errors.Is(err, renderErr) {
		t.Fatalf("Run() error = %v, want render error", err)
	}
	if fake.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
	}
}

func TestApplicationRunReportsStartupFailures(t *testing.T) {
	t.Run("address", func(t *testing.T) {
		application, _, _, path := newTestApplication(t)
		want := errors.New("address failed")
		application.advertiseAddress = func() (netip.Addr, error) { return netip.Addr{}, want }
		if err := application.Run(context.Background(), Request{Path: path, Lifetime: time.Hour}); !errors.Is(err, want) {
			t.Fatalf("Run() error = %v", err)
		}
	})
	t.Run("listen", func(t *testing.T) {
		application, fake, _, path := newTestApplication(t)
		want := errors.New("listen failed")
		fake.startErr = want
		if err := application.Run(context.Background(), Request{Path: path, Lifetime: time.Hour}); !errors.Is(err, want) {
			t.Fatalf("Run() error = %v", err)
		}
	})
	t.Run("invalid listen address", func(t *testing.T) {
		application, fake, _, path := newTestApplication(t)
		fake.addr = testAddr("invalid")
		if err := application.Run(context.Background(), Request{Path: path, Lifetime: time.Hour}); err == nil {
			t.Fatal("Run() error = nil")
		}
		if fake.closeCalls != 1 {
			t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
		}
	})
}

func newTestApplication(t *testing.T) (*Application, *fakeSessionServer, *bytes.Buffer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shared.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	fake := &fakeSessionServer{done: make(chan error, 1), addr: testAddr("192.0.2.10:55544")}
	application := New(Dependencies{Stderr: stderr})
	application.advertiseAddress = func() (netip.Addr, error) { return netip.MustParseAddr("192.0.2.10"), nil }
	application.newServer = func(*session.Session) sessionServer { return fake }
	application.renderQR = func(dst io.Writer, payload string) error { _, err := io.WriteString(dst, "QR:"+payload); return err }
	return application, fake, stderr, path
}

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type fakeSessionServer struct {
	fakeShutdownServer
	done     chan error
	addr     net.Addr
	startErr error
}

func (s *fakeSessionServer) Start(string) (net.Addr, error) { return s.addr, s.startErr }
func (s *fakeSessionServer) Done() <-chan error             { return s.done }

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
	shutdown      func(context.Context) error
	closeErr      error
	closeCalls    int
	shutdownCalls int
}

func (s *fakeShutdownServer) Shutdown(ctx context.Context) error {
	s.shutdownCalls++
	if s.shutdown == nil {
		return nil
	}
	return s.shutdown(ctx)
}

func (s *fakeShutdownServer) Close() error {
	s.closeCalls++
	return s.closeErr
}
