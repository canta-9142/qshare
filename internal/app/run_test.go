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
	"syscall"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/platform/firewall"
	platformnetwork "github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestApplicationDirectoryValidationPrecedesServerCreation(t *testing.T) {
	application := New(Dependencies{Stderr: io.Discard})
	want := errors.New("walk failed")
	application.openDirectory = func(string) (*share.Directory, error) { return nil, want }
	created := false
	application.newDirectoryServer = func(*session.Session) sessionServer { created = true; return nil }
	err := application.Run(context.Background(), Request{Operation: OperationSendDirectory, Paths: []string{"dir"}, Lifetime: time.Hour})
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v", err)
	}
	if created {
		t.Fatal("server created after directory validation failure")
	}
}

func TestApplicationRunExpiration(t *testing.T) {
	application, fake, stderr, path := newTestApplication(t)
	lease := &fakeFirewallLease{}
	application.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return lease, nil
	}
	err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Millisecond})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fake.shutdownCalls != 1 || fake.closeCalls != 0 {
		t.Fatalf("shutdown=%d close=%d", fake.shutdownCalls, fake.closeCalls)
	}
	if got := stderr.String(); !strings.Contains(got, "http://192.0.2.10:55544/s/") || !strings.Contains(got, "This URL expires after 1ms") {
		t.Fatalf("stderr = %q", got)
	}
	if lease.closeCalls != 1 {
		t.Errorf("firewall Close() calls = %d, want 1", lease.closeCalls)
	}
}

func TestApplicationRunCancellation(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	cause := errors.New("cancelled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cause", err)
	}
	if fake.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
	}
}

func TestApplicationRunInteractiveShutdown(t *testing.T) {
	application, fake, stderr, path := newTestApplication(t)
	shutdownRequested := make(chan struct{})
	close(shutdownRequested)
	qrRendered := false
	application.renderQR = func(io.Writer, string) error {
		qrRendered = true
		return nil
	}
	application.startShutdownListener = func() (<-chan struct{}, error) {
		if !qrRendered {
			t.Fatal("shutdown listener started before QR rendering completed")
		}
		return shutdownRequested, nil
	}

	err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fake.shutdownCalls != 1 || fake.closeCalls != 0 {
		t.Fatalf("shutdown=%d close=%d, want shutdown=1 close=0", fake.shutdownCalls, fake.closeCalls)
	}
	if got := stderr.String(); !strings.Contains(got, "Press q to quit.") {
		t.Fatalf("stderr = %q, want quit hint", got)
	}
}

func TestApplicationInteractiveShutdownCanBeInterrupted(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	shutdownRequested := make(chan struct{})
	close(shutdownRequested)
	application.startShutdownListener = func() (<-chan struct{}, error) {
		return shutdownRequested, nil
	}
	shutdownStarted := make(chan struct{})
	fake.shutdown = func(ctx context.Context) error {
		close(shutdownStarted)
		<-ctx.Done()
		return context.Cause(ctx)
	}

	cause := errors.New("interrupt graceful shutdown")
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- application.Run(ctx, Request{Paths: []string{path}, Lifetime: time.Hour})
	}()

	<-shutdownStarted
	cancel(cause)
	if err := <-done; !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", err)
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
	err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, serveErr) {
		t.Fatalf("Run() error = %v, want server error", err)
	}
	if fake.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
	}
}

func TestApplicationRunClosesServerWhenQRRenderingFails(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	lease := &fakeFirewallLease{}
	application.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return lease, nil
	}
	renderErr := errors.New("render failed")
	application.renderQR = func(io.Writer, string) error { return renderErr }
	listenerStarted := false
	application.startShutdownListener = func() (<-chan struct{}, error) {
		listenerStarted = true
		return nil, nil
	}
	err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, renderErr) {
		t.Fatalf("Run() error = %v, want render error", err)
	}
	if fake.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
	}
	if lease.closeCalls != 1 {
		t.Errorf("firewall Close() calls = %d, want 1", lease.closeCalls)
	}
	if listenerStarted {
		t.Fatal("shutdown listener started after QR rendering failed")
	}
}

func TestApplicationClosesSessionWhenShutdownListenerFails(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	lease := &fakeFirewallLease{}
	application.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return lease, nil
	}
	want := errors.New("terminal failed")
	application.startShutdownListener = func() (<-chan struct{}, error) {
		return nil, want
	}

	err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want shutdown listener error", err)
	}
	if fake.closeCalls != 1 {
		t.Errorf("server Close() calls = %d, want 1", fake.closeCalls)
	}
	if lease.closeCalls != 1 {
		t.Errorf("firewall Close() calls = %d, want 1", lease.closeCalls)
	}
}

func TestApplicationOpensFirewallBeforeRenderingAndClosesItWithServer(t *testing.T) {
	application, _, _, path := newTestApplication(t)
	lease := &fakeFirewallLease{}
	var gotRule firewall.Rule
	opened := false
	application.openFirewall = func(_ context.Context, rule firewall.Rule) (firewallLease, error) {
		opened = true
		gotRule = rule
		return lease, nil
	}
	application.renderQR = func(io.Writer, string) error {
		if !opened {
			t.Fatal("QR rendered before firewall was configured")
		}
		return nil
	}

	cause := errors.New("stop")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", err)
	}
	if gotRule.Interface != "eth0" || gotRule.Source != netip.MustParsePrefix("192.0.2.0/24") || gotRule.Destination != netip.MustParseAddr("192.0.2.10") || gotRule.Port != 55544 {
		t.Errorf("firewall rule = %+v", gotRule)
	}
	minimumTimeout := time.Hour + expirationDrainTimeout
	maximumTimeout := minimumTimeout + firewallTimeoutSlack
	if gotRule.Timeout < minimumTimeout || gotRule.Timeout > maximumTimeout {
		t.Errorf("firewall timeout = %v, want between %v and %v", gotRule.Timeout, minimumTimeout, maximumTimeout)
	}
	if lease.closeCalls != 1 {
		t.Errorf("firewall Close() calls = %d, want 1", lease.closeCalls)
	}
}

func TestApplicationFirewallFailureClosesServerBeforeRendering(t *testing.T) {
	application, fake, _, path := newTestApplication(t)
	want := errors.New("firewall failed")
	application.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return nil, want
	}
	rendered := false
	application.renderQR = func(io.Writer, string) error {
		rendered = true
		return nil
	}

	err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want firewall error", err)
	}
	if rendered {
		t.Fatal("QR rendered after firewall configuration failed")
	}
	if fake.closeCalls != 1 {
		t.Errorf("server Close() calls = %d, want 1", fake.closeCalls)
	}
}

func TestApplicationReportsFirewallCleanupFailure(t *testing.T) {
	application, _, _, path := newTestApplication(t)
	cleanupErr := errors.New("firewall cleanup failed")
	application.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return &fakeFirewallLease{closeErr: cleanupErr}, nil
	}

	cause := errors.New("stop")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, cause) || !errors.Is(err, cleanupErr) {
		t.Fatalf("Run() error = %v, want cancellation and cleanup errors", err)
	}
}

func TestApplicationRunReportsStartupFailures(t *testing.T) {
	t.Run("address", func(t *testing.T) {
		application, _, _, path := newTestApplication(t)
		want := errors.New("address failed")
		application.advertiseEndpoint = func() (platformnetwork.Endpoint, error) {
			return platformnetwork.Endpoint{}, want
		}
		if err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour}); !errors.Is(err, want) {
			t.Fatalf("Run() error = %v", err)
		}
	})
	t.Run("listen", func(t *testing.T) {
		application, fake, _, path := newTestApplication(t)
		want := errors.New("listen failed")
		fake.startErr = want
		if err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour}); !errors.Is(err, want) {
			t.Fatalf("Run() error = %v", err)
		}
	})
	t.Run("invalid listen address", func(t *testing.T) {
		application, fake, _, path := newTestApplication(t)
		fake.addr = testAddr("invalid")
		if err := application.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour}); err == nil {
			t.Fatal("Run() error = nil")
		}
		if fake.closeCalls != 1 {
			t.Fatalf("Close() calls = %d, want 1", fake.closeCalls)
		}
	})
}

func TestApplicationRetriesRandomPortWhenCandidateIsInUse(t *testing.T) {
	application, fake, stderr, path := newTestApplication(t)
	var firewallPort uint16
	application.openFirewall = func(_ context.Context, rule firewall.Rule) (firewallLease, error) {
		firewallPort = rule.Port
		return &fakeFirewallLease{}, nil
	}
	application.selectServerPort = func() (uint16, error) { return 59999, nil }
	fake.start = func(bindAddr string) (net.Addr, error) {
		if len(fake.startAddrs) == 1 {
			return nil, syscall.EADDRINUSE
		}
		return testAddr(bindAddr), nil
	}

	cause := errors.New("stop")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{Paths: []string{path}, Lifetime: time.Hour})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation", err)
	}
	if len(fake.startAddrs) != 2 ||
		fake.startAddrs[0] != "192.0.2.10:59999" ||
		fake.startAddrs[1] != "192.0.2.10:50000" {
		t.Fatalf("Start() addresses = %v", fake.startAddrs)
	}
	if !strings.Contains(stderr.String(), "http://192.0.2.10:50000/s/") {
		t.Fatalf("stderr = %q, want selected port", stderr.String())
	}
	if firewallPort != 50000 {
		t.Fatalf("firewall port = %d, want 50000", firewallPort)
	}
}

func TestRandomServerPortIsInConfiguredRange(t *testing.T) {
	for range 100 {
		port, err := randomServerPort()
		if err != nil {
			t.Fatalf("randomServerPort() error = %v", err)
		}
		if port < minimumServerPort || port >= minimumServerPort+serverPortCount {
			t.Fatalf("randomServerPort() = %d, want %d-%d", port, minimumServerPort, minimumServerPort+serverPortCount-1)
		}
	}
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
	configureTestNetworking(application)
	application.selectServerPort = func() (uint16, error) { return 55544, nil }
	application.newSendServer = func(*session.Session) sessionServer { return fake }
	application.renderQR = func(dst io.Writer, payload string) error { _, err := io.WriteString(dst, "QR:"+payload); return err }
	return application, fake, stderr, path
}

func configureTestNetworking(application *Application) *fakeFirewallLease {
	application.advertiseEndpoint = func() (platformnetwork.Endpoint, error) {
		return platformnetwork.Endpoint{
			Address:   netip.MustParseAddr("192.0.2.10"),
			Prefix:    netip.MustParsePrefix("192.0.2.0/24"),
			Interface: "eth0",
		}, nil
	}
	lease := &fakeFirewallLease{}
	application.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return lease, nil
	}
	return lease
}

type fakeFirewallLease struct {
	closeCalls int
	closeErr   error
}

func (l *fakeFirewallLease) Close(context.Context) error {
	l.closeCalls++
	return l.closeErr
}

func TestApplicationRunReceiveMode(t *testing.T) {
	stderr := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	fake := &fakeSessionServer{done: make(chan error, 1), addr: testAddr("192.0.2.10:55544")}
	application := New(Dependencies{Stdout: stdout, Stderr: stderr})
	configureTestNetworking(application)

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
	application.newReceiveServer = func(_ *session.Session, got receiveStore, submitter textSubmitter) sessionServer {
		serverReceivedStore = got != nil
		text, err := share.NewText([]byte("received text"))
		if err != nil {
			t.Fatal(err)
		}
		if err := submitter.Submit(context.Background(), text); err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
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
	if got := stdout.String(); got != "received text" {
		t.Errorf("stdout = %q, want received text", got)
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

func TestApplicationInteractiveShutdownDrainsReceivedText(t *testing.T) {
	shutdownRequested := make(chan struct{})
	close(shutdownRequested)
	fake := &fakeSessionServer{done: make(chan error, 1), addr: testAddr("192.0.2.10:55544")}
	application := New(Dependencies{
		Stderr: io.Discard,
		StartShutdownListener: func() (<-chan struct{}, error) {
			return shutdownRequested, nil
		},
	})
	configureTestNetworking(application)
	application.openReceiveStore = func(string) (receiveStore, error) {
		return receiveStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
			return receive.Result{}, nil
		}), nil
	}

	sinkStarted := make(chan struct{})
	releaseSink := make(chan struct{})
	var received string
	application.newClipboardSink = func(string) (receive.TextSink, error) {
		return textSinkFunc(func(_ context.Context, text share.Text) error {
			close(sinkStarted)
			<-releaseSink
			received = text.String()
			return nil
		}), nil
	}
	submitDone := make(chan error, 1)
	application.newReceiveServer = func(_ *session.Session, _ receiveStore, submitter textSubmitter) sessionServer {
		text, err := share.NewText([]byte("drained text"))
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			submitDone <- submitter.Submit(context.Background(), text)
		}()
		<-sinkStarted
		return fake
	}
	application.renderQR = func(io.Writer, string) error { return nil }

	runDone := make(chan error, 1)
	go func() {
		runDone <- application.Run(context.Background(), Request{
			Operation:  OperationReceive,
			ReceiveDir: "/receive",
			Clipboard:  "xclip",
			Lifetime:   time.Hour,
		})
	}()

	select {
	case err := <-runDone:
		t.Fatalf("Run() returned before text drain: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseSink)
	if err := <-submitDone; err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if received != "drained text" {
		t.Fatalf("received text = %q, want drained text", received)
	}
	if fake.shutdownCalls != 1 || fake.closeCalls != 0 {
		t.Fatalf("shutdown=%d close=%d, want shutdown=1 close=0", fake.shutdownCalls, fake.closeCalls)
	}
}

func TestApplicationRunReceiveModeUsesClipboardSink(t *testing.T) {
	var clipboard bytes.Buffer
	fake := &fakeSessionServer{done: make(chan error, 1), addr: testAddr("192.0.2.10:55544")}
	application := New(Dependencies{Stdout: io.Discard, Stderr: io.Discard})
	configureTestNetworking(application)
	application.openReceiveStore = func(string) (receiveStore, error) {
		return receiveStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
			return receive.Result{}, nil
		}), nil
	}
	var selectedBackend string
	application.newClipboardSink = func(backend string) (receive.TextSink, error) {
		selectedBackend = backend
		return receive.NewWriterTextSink(&clipboard), nil
	}
	application.newReceiveServer = func(_ *session.Session, _ receiveStore, submitter textSubmitter) sessionServer {
		text, err := share.NewText([]byte("clipboard value"))
		if err != nil {
			t.Fatal(err)
		}
		if err := submitter.Submit(context.Background(), text); err != nil {
			t.Fatal(err)
		}
		return fake
	}
	application.renderQR = func(io.Writer, string) error { return nil }

	cause := errors.New("stop clipboard test")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{
		Operation:  OperationReceive,
		ReceiveDir: "/receive",
		Clipboard:  "xclip",
		Lifetime:   time.Hour,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", err)
	}
	if selectedBackend != "xclip" {
		t.Errorf("selected backend = %q, want xclip", selectedBackend)
	}
	if got := clipboard.String(); got != "clipboard value" {
		t.Errorf("clipboard sink = %q, want clipboard value", got)
	}
}

func TestApplicationAutoClipboardMissingFallsBackToStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	fake := &fakeSessionServer{done: make(chan error, 1), addr: testAddr("192.0.2.10:55544")}
	application := New(Dependencies{Stdout: &stdout, Stderr: &stderr})
	configureTestNetworking(application)
	application.openReceiveStore = func(string) (receiveStore, error) {
		return receiveStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
			return receive.Result{}, nil
		}), nil
	}
	application.newClipboardSink = func(backend string) (receive.TextSink, error) {
		if backend != "auto" {
			t.Fatalf("clipboard backend = %q, want auto", backend)
		}
		return nil, clipboard.ErrBackendNotFound
	}
	application.newReceiveServer = func(_ *session.Session, _ receiveStore, submitter textSubmitter) sessionServer {
		text, err := share.NewText([]byte("fallback value"))
		if err != nil {
			t.Fatal(err)
		}
		if err := submitter.Submit(context.Background(), text); err != nil {
			t.Fatal(err)
		}
		return fake
	}
	application.renderQR = func(io.Writer, string) error { return nil }

	cause := errors.New("stop auto clipboard test")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := application.Run(ctx, Request{
		Operation:  OperationReceive,
		ReceiveDir: "/receive",
		Clipboard:  "auto",
		Lifetime:   time.Hour,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", err)
	}
	if got := stdout.String(); got != "fallback value" {
		t.Errorf("stdout = %q, want fallback value", got)
	}
	if got := stderr.String(); !strings.Contains(got, "Clipboard backend not found") {
		t.Errorf("stderr = %q, want missing-backend notice", got)
	}
}

func TestApplicationClipboardConfigurationFailurePreventsServerStart(t *testing.T) {
	application := New(Dependencies{Stdout: io.Discard, Stderr: io.Discard})
	application.openReceiveStore = func(string) (receiveStore, error) {
		return receiveStoreFunc(func(context.Context, string, io.Reader) (receive.Result, error) {
			return receive.Result{}, nil
		}), nil
	}
	want := errors.New("backend not found")
	application.newClipboardSink = func(string) (receive.TextSink, error) {
		return nil, want
	}
	serverCreated := false
	application.newReceiveServer = func(*session.Session, receiveStore, textSubmitter) sessionServer {
		serverCreated = true
		return nil
	}

	err := application.Run(context.Background(), Request{
		Operation:  OperationReceive,
		ReceiveDir: "/receive",
		Clipboard:  "wl-copy",
		Lifetime:   time.Hour,
	})
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want backend error", err)
	}
	if serverCreated {
		t.Fatal("receive server was created after clipboard configuration failure")
	}
}

func TestApplicationUnsupportedClipboardBackendIsInvalidRequest(t *testing.T) {
	application := New(Dependencies{Stdout: io.Discard, Stderr: io.Discard})
	storeOpened := false
	application.openReceiveStore = func(string) (receiveStore, error) {
		storeOpened = true
		return nil, errors.New("unexpected receive store open")
	}

	err := application.Run(context.Background(), Request{
		Operation:  OperationReceive,
		ReceiveDir: "/receive",
		Clipboard:  "unsupported",
		Lifetime:   time.Hour,
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Run() error = %v, want ErrInvalidRequest", err)
	}
	if storeOpened {
		t.Fatal("receive store was opened for an unsupported clipboard backend")
	}
}

func TestApplicationRunTextSendMode(t *testing.T) {
	stderr := &bytes.Buffer{}
	fake := &fakeSessionServer{done: make(chan error, 1), addr: testAddr("192.0.2.10:55544")}
	application := New(Dependencies{Stderr: stderr})
	configureTestNetworking(application)

	text, err := share.NewText([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	var serverText string
	application.newTextServer = func(sess *session.Session) sessionServer {
		got, ok := sess.Text()
		if ok {
			serverText = got.String()
		}
		return fake
	}
	var qrPayload string
	application.renderQR = func(_ io.Writer, payload string) error {
		qrPayload = payload
		return nil
	}

	cause := errors.New("stop text test")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err = application.Run(ctx, Request{
		Operation: OperationSendText,
		Text:      text,
		Lifetime:  time.Hour,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", err)
	}
	if serverText != "hello" {
		t.Errorf("server text = %q, want hello", serverText)
	}
	if qrPayload == "" || !strings.HasPrefix(qrPayload, "http://192.0.2.10:55544/s/") {
		t.Errorf("QR payload = %q", qrPayload)
	}
	if got := stderr.String(); !strings.Contains(got, "Sharing text") {
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
	application.newReceiveServer = func(*session.Session, receiveStore, textSubmitter) sessionServer {
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

type textSinkFunc func(context.Context, share.Text) error

func (function textSinkFunc) WriteText(ctx context.Context, text share.Text) error {
	return function(ctx, text)
}

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type fakeSessionServer struct {
	fakeShutdownServer
	done       chan error
	addr       net.Addr
	startErr   error
	start      func(string) (net.Addr, error)
	startAddrs []string
}

func (s *fakeSessionServer) Start(bindAddr string) (net.Addr, error) {
	s.startAddrs = append(s.startAddrs, bindAddr)
	if s.start != nil {
		return s.start(bindAddr)
	}
	return s.addr, s.startErr
}

func (s *fakeSessionServer) Done() <-chan error { return s.done }

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
