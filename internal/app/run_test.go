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

	"github.com/canta-9142/qshare/internal/receive"
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
	application.newSendServer = func(*session.Session) sessionServer { return fake }
	application.renderQR = func(dst io.Writer, payload string) error { _, err := io.WriteString(dst, "QR:"+payload); return err }
	return application, fake, stderr, path
}

func TestApplicationRunReceiveMode(t *testing.T) {
	stderr := &bytes.Buffer{}
	fake := &fakeSessionServer{done: make(chan error, 1), addr: testAddr("192.0.2.10:55544")}
	application := New(Dependencies{Stderr: stderr})
	application.advertiseAddress = func() (netip.Addr, error) {
		return netip.MustParseAddr("192.0.2.10"), nil
	}

	receiveDir := filepath.Join(t.TempDir(), "received")
	var openedDir string
	store := receiveStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
		return receive.Result{}, nil
	})
	application.openReceiveStore = func(dir string) (receiveStore, error) {
		openedDir = dir
		return store, nil
	}
	serverReceivedStore := false
	application.newReceiveServer = func(_ *session.Session, got receiveStore) sessionServer {
		serverReceivedStore = got != nil
		return fake
	}
	var qrPayload string
	application.renderQR = func(_ io.Writer, payload string) error {
		qrPayload = payload
		return nil
	}

	cause := errors.New("stop receive test")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{
		Operation:  OperationReceive,
		ReceiveDir: receiveDir,
		Lifetime:   time.Hour,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", err)
	}
	if openedDir != receiveDir {
		t.Errorf("opened receive directory = %q, want %q", openedDir, receiveDir)
	}
	if !serverReceivedStore {
		t.Fatal("receive server did not receive opened store")
	}
	if qrPayload == "" || !strings.HasPrefix(qrPayload, "http://192.0.2.10:55544/s/") {
		t.Errorf("QR payload = %q", qrPayload)
	}
	if got := stderr.String(); !strings.Contains(got, "Receiving into "+receiveDir) {
		t.Errorf("stderr = %q", got)
	}
	if fake.closeCalls != 1 {
		t.Errorf("Close() calls = %d, want 1", fake.closeCalls)
	}
}

func TestApplicationReceiveStoreFailurePreventsServerStart(t *testing.T) {
	application := New(Dependencies{Stderr: io.Discard})
	want := errors.New("store failed")
	application.openReceiveStore = func(string) (receiveStore, error) {
		return nil, want
	}
	serverCreated := false
	application.newReceiveServer = func(*session.Session, receiveStore) sessionServer {
		serverCreated = true
		return nil
	}

	err := application.Run(context.Background(), Request{
		Operation:  OperationReceive,
		ReceiveDir: "/receive",
		Lifetime:   time.Hour,
	})
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want store error", err)
	}
	if serverCreated {
		t.Fatal("receive server was created after store failure")
	}
}

type receiveStoreFunc func(context.Context, string, io.Reader) (receive.Result, error)

func (function receiveStoreFunc) Save(ctx context.Context, name string, source io.Reader) (receive.Result, error) {
	return function(ctx, name, source)
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
