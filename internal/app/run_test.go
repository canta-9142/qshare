package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/platform/firewall"
	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/share"
)

func TestApplicationModes(t *testing.T) {
	for _, mode := range []string{"files", "directory", "text", "receive", "clipboard", "auto fallback"} {
		t.Run(mode, func(t *testing.T) {
			a, listener, stderr, path := newTestApplication(t)
			var stdout bytes.Buffer
			a.stdout = &stdout
			req := Request{Paths: []string{path}, Lifetime: time.Hour}
			wantPage := "shared.txt"
			switch mode {
			case "directory":
				req.Operation, req.Paths = OperationSendDirectory, []string{filepath.Dir(path)}
			case "text":
				req.Operation = OperationSendText
				req.Text, _ = share.NewText([]byte("hello text"))
				wantPage = "hello text"
			case "receive", "clipboard", "auto fallback":
				req.Operation, req.Paths = OperationReceive, nil
				req.ReceiveDir = t.TempDir()
				wantPage = ""
				if mode == "clipboard" {
					req.Clipboard = "xclip"
					a.newClipboardSink = func(backend string) (receive.TextSink, error) {
						if backend != "xclip" {
							t.Errorf("backend = %q", backend)
						}
						return receive.NewWriterTextSink(&stdout), nil
					}
				}
				if mode == "auto fallback" {
					req.Clipboard = "auto"
					a.newClipboardSink = func(string) (receive.TextSink, error) { return nil, clipboard.ErrBackendNotFound }
				}
			}
			client := &http.Client{Timeout: 2 * time.Second}
			qrRendered := false
			a.renderQR = func(_ io.Writer, payload string) error {
				qrRendered = true
				if !strings.HasPrefix(payload, "http://192.0.2.10:55544/s/") {
					t.Errorf("advertised URL = %q", payload)
				}
				remote := localURL(t, listener, payload)
				response, err := client.Get(remote)
				if err != nil {
					return err
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					return err
				}
				if response.StatusCode != http.StatusOK || !strings.Contains(string(body), wantPage) {
					t.Errorf("page status=%d body=%q", response.StatusCode, body)
				}
				if req.Operation == OperationReceive {
					u, _ := url.Parse(remote)
					token := strings.TrimPrefix(u.Path, "/s/")
					u.Path = "/t/" + token
					response, err = client.Post(u.String(), "text/plain", strings.NewReader("received text"))
					if err != nil {
						return err
					}
					response.Body.Close()
					if response.StatusCode != http.StatusNoContent {
						t.Errorf("submit status = %d", response.StatusCode)
					}
					var upload bytes.Buffer
					form := multipart.NewWriter(&upload)
					part, err := form.CreateFormFile("file", "uploaded.txt")
					if err != nil {
						return err
					}
					io.WriteString(part, "uploaded content")
					form.Close()
					u.Path = "/u/" + token
					response, err = client.Post(u.String(), form.FormDataContentType(), &upload)
					if err != nil {
						return err
					}
					response.Body.Close()
					if response.StatusCode != http.StatusCreated {
						t.Errorf("upload status = %d", response.StatusCode)
					}
				}
				return nil
			}
			restored := false
			a.startShutdownListener = func() (<-chan struct{}, func() error, error) {
				if !qrRendered {
					t.Error("terminal initialized before QR")
				}
				quit := make(chan struct{})
				close(quit)
				return quit, func() error { restored = true; return nil }, nil
			}
			if err := a.Run(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			awaitLifecycle(t, listener.closed)
			if !restored || !strings.Contains(stderr.String(), "Press q to quit.") {
				t.Error("interactive shutdown did not restore the terminal or print the quit hint")
			}
			if req.Operation == OperationReceive {
				if stdout.String() != "received text" {
					t.Errorf("received text = %q", stdout.String())
				}
				data, err := os.ReadFile(filepath.Join(req.ReceiveDir, "uploaded.txt"))
				if err != nil || string(data) != "uploaded content" {
					t.Errorf("uploaded file = %q, error=%v", data, err)
				}
			}
			if mode == "auto fallback" && !strings.Contains(stderr.String(), "Clipboard backend not found") {
				t.Error("missing clipboard fallback notice")
			}
		})
	}
}

func TestApplicationStartupFailures(t *testing.T) {
	for _, stage := range []string{"files", "directory", "session", "clipboard", "receive store", "address", "port", "listen", "firewall", "QR", "terminal", "serve"} {
		t.Run(stage, func(t *testing.T) {
			a, listener, _, path := newTestApplication(t)
			want := errors.New(stage + " failed")
			req := Request{Paths: []string{path}, Lifetime: time.Hour}
			bound, firewallOpened, terminalStarted := false, false, false
			a.listen = func(string, string) (net.Listener, error) { bound = true; return listener, nil }
			firewallClosed := false
			a.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
				firewallOpened = true
				return firewallLeaseFunc(func(ctx context.Context) error {
					select {
					case <-listener.closed:
					default:
						t.Error("firewall removed before listener closed")
					}
					if ctx.Err() != nil {
						t.Errorf("cleanup context: %v", ctx.Err())
					}
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > firewallCleanupTimeout {
						t.Error("firewall cleanup deadline missing")
					}
					firewallClosed = true
					return nil
				}), nil
			}
			a.startShutdownListener = func() (<-chan struct{}, func() error, error) {
				terminalStarted = true
				return nil, nil, nil
			}
			switch stage {
			case "files":
				a.openCollection = func([]string) (*share.Collection, error) { return nil, want }
			case "directory":
				req.Operation = OperationSendDirectory
				a.openDirectory = func(string) (*share.Directory, error) { return nil, want }
			case "session":
				req.Lifetime = 0
			case "clipboard":
				req.Operation, req.Clipboard = OperationReceive, "wl-copy"
				a.newClipboardSink = func(string) (receive.TextSink, error) { return nil, want }
			case "receive store":
				req.Operation = OperationReceive
				a.openReceiveStore = func(string) (*receive.Store, error) { return nil, want }
			case "address":
				a.advertiseEndpoint = func() (network.Endpoint, error) { return network.Endpoint{}, want }
			case "port":
				a.selectServerPort = func() (uint16, error) { return 0, want }
			case "listen":
				a.listen = func(string, string) (net.Listener, error) { return nil, want }
			case "firewall":
				a.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) { return nil, want }
			case "QR":
				a.renderQR = func(io.Writer, string) error { return want }
			case "terminal":
				a.startShutdownListener = func() (<-chan struct{}, func() error, error) { return nil, nil, want }
			case "serve":
				listener.acceptErr = want
			}
			err := a.Run(context.Background(), req)
			if err == nil || (stage != "session" && !errors.Is(err, want)) {
				t.Fatalf("Run() error = %v, want %v", err, want)
			}
			if bound {
				awaitLifecycle(t, listener.closed)
			}
			if firewallOpened != firewallClosed {
				t.Error("acquired firewall lease was not released")
			}
			if terminalStarted && stage != "serve" {
				t.Error("terminal initialized after startup failure")
			}
		})
	}
}

func TestApplicationExpirationAndCancellation(t *testing.T) {
	for _, expired := range []bool{true, false} {
		a, listener, _, path := newTestApplication(t)
		ctx, cancel := context.WithCancelCause(context.Background())
		cause := errors.New("signal")
		lifetime := time.Nanosecond
		if !expired {
			lifetime = time.Hour
			cancel(cause)
		}
		err := a.Run(ctx, Request{Paths: []string{path}, Lifetime: lifetime})
		cancel(nil)
		if expired && err != nil {
			t.Fatalf("expiration error = %v", err)
		}
		if !expired && !errors.Is(err, cause) {
			t.Fatalf("cancellation error = %v", err)
		}
		awaitLifecycle(t, listener.closed)
	}
}

func TestApplicationUnsupportedClipboardBackendIsInvalidRequest(t *testing.T) {
	a := New(Dependencies{Stderr: io.Discard})
	a.openReceiveStore = func(string) (*receive.Store, error) {
		t.Error("receive store opened for an unsupported backend")
		return nil, nil
	}
	err := a.Run(context.Background(), Request{Operation: OperationReceive, Clipboard: "unsupported", Lifetime: time.Hour})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestApplicationRetriesPortAndUsesSelectedPort(t *testing.T) {
	a, listener, _, path := newTestApplication(t)
	a.selectServerPort = func() (uint16, error) { return 59999, nil }
	var addresses []string
	a.listen = func(network, address string) (net.Listener, error) {
		if network != "tcp" {
			t.Errorf("network = %q", network)
		}
		addresses = append(addresses, address)
		if len(addresses) == 1 {
			return nil, syscall.EADDRINUSE
		}
		return listener, nil
	}
	var rule firewall.Rule
	a.openFirewall = func(_ context.Context, got firewall.Rule) (firewallLease, error) {
		rule = got
		return firewallLeaseFunc(func(context.Context) error { return nil }), nil
	}
	a.renderQR = func(_ io.Writer, payload string) error {
		if !strings.HasPrefix(payload, "http://192.0.2.10:50000/s/") {
			t.Errorf("URL = %q", payload)
		}
		return nil
	}
	quit := make(chan struct{})
	close(quit)
	a.startShutdownListener = func() (<-chan struct{}, func() error, error) { return quit, nil, nil }
	if err := a.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(addresses, ",") != "192.0.2.10:59999,192.0.2.10:50000" {
		t.Errorf("listen addresses = %v", addresses)
	}
	if rule.Port != 50000 || rule.Interface != "eth0" || rule.Source != netip.MustParsePrefix("192.0.2.0/24") || rule.Destination != netip.MustParseAddr("192.0.2.10") {
		t.Errorf("firewall rule = %+v", rule)
	}
	if rule.Timeout < time.Hour+expirationDrainTimeout || rule.Timeout > time.Hour+expirationDrainTimeout+firewallTimeoutSlack {
		t.Errorf("firewall timeout = %v", rule.Timeout)
	}
}

func TestListenLANFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		port      uint16
		listenErr error
		attempts  int
	}{
		{"range low", 49999, nil, 0},
		{"range high", 60000, nil, 0},
		{"occupied", 59999, syscall.EADDRINUSE, serverPortAttempts},
		{"permission", 55544, syscall.EACCES, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := New(Dependencies{Stderr: io.Discard})
			a.selectServerPort = func() (uint16, error) { return tc.port, nil }
			addresses := make(map[string]bool)
			a.listen = func(_, address string) (net.Listener, error) {
				if addresses[address] {
					t.Errorf("port retried twice: %s", address)
				}
				addresses[address] = true
				return nil, tc.listenErr
			}
			ln, _, err := a.listenLAN(network.Endpoint{Address: netip.MustParseAddr("127.0.0.1")})
			if err == nil || ln != nil || len(addresses) != tc.attempts {
				t.Fatalf("listener=%v error=%v attempts=%d", ln, err, len(addresses))
			}
			if tc.listenErr != nil && !errors.Is(err, tc.listenErr) {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestListenLANBindsSelectedPort(t *testing.T) {
	a := New(Dependencies{Stderr: io.Discard})
	listener, port, err := a.listenLAN(network.Endpoint{Address: netip.MustParseAddr("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr)
	if address.Port != int(port) || !address.IP.Equal(net.ParseIP("127.0.0.1")) ||
		port < minimumServerPort || port >= minimumServerPort+serverPortCount {
		t.Fatalf("listener address=%v selected port=%d", address, port)
	}
}

func TestRandomServerPortIsInConfiguredRange(t *testing.T) {
	for range 100 {
		port, err := randomServerPort()
		if err != nil || port < minimumServerPort || port >= minimumServerPort+serverPortCount {
			t.Fatalf("port=%d error=%v", port, err)
		}
	}
}

// The injected listener uses an ephemeral loopback port. Production still binds
// the selected LAN address and port; tests translate only the destination host.
func newTestApplication(t *testing.T) (*Application, *testListener, *bytes.Buffer, string) {
	t.Helper()
	listener := newTestListener(t)
	path := filepath.Join(t.TempDir(), "shared.txt")
	if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	a := New(Dependencies{Stderr: stderr})
	a.advertiseEndpoint = func() (network.Endpoint, error) {
		return network.Endpoint{Address: netip.MustParseAddr("192.0.2.10"), Prefix: netip.MustParsePrefix("192.0.2.0/24"), Interface: "eth0"}, nil
	}
	a.selectServerPort = func() (uint16, error) { return 55544, nil }
	a.listen = func(string, string) (net.Listener, error) { return listener, nil }
	a.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return firewallLeaseFunc(func(context.Context) error { return nil }), nil
	}
	a.renderQR = func(io.Writer, string) error { return nil }
	var file *share.File
	a.openCollection = func(paths []string) (*share.Collection, error) {
		files, err := share.OpenCollection(paths)
		if err == nil {
			file = files.Resources()[0].File()
		}
		return files, err
	}
	t.Cleanup(func() {
		if file != nil {
			assertFileClosed(t, file)
		}
	})
	return a, listener, stderr, path
}

type testListener struct {
	net.Listener
	closed    chan struct{}
	once      sync.Once
	acceptErr error
	closeErr  error
	onClose   func()
}

func newTestListener(t *testing.T) *testListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return &testListener{Listener: ln, closed: make(chan struct{})}
}

func (l *testListener) Accept() (net.Conn, error) {
	if l.acceptErr != nil {
		return nil, l.acceptErr
	}
	return l.Listener.Accept()
}

func (l *testListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() {
		if l.onClose != nil {
			l.onClose()
		}
		close(l.closed)
	})
	return errors.Join(err, l.closeErr)
}

type firewallLeaseFunc func(context.Context) error

func (f firewallLeaseFunc) Close(ctx context.Context) error { return f(ctx) }

type textSinkFunc func(context.Context, share.Text) error

func (f textSinkFunc) WriteText(ctx context.Context, text share.Text) error { return f(ctx, text) }

func localURL(t *testing.T, listener net.Listener, payload string) string {
	t.Helper()
	u, err := url.Parse(payload)
	if err != nil {
		t.Fatal(err)
	}
	u.Host = listener.Addr().String()
	return u.String()
}
