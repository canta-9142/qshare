package app

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/platform/firewall"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestApplicationCleanupContinuesAfterErrors(t *testing.T) {
	a, server, _, path := newTestApplication(t)
	var file *share.File
	a.openCollection = func(paths []string) (*share.Collection, error) {
		files, err := share.OpenCollection(paths)
		if err == nil {
			file = files.Resources()[0].File()
		}
		return files, err
	}
	shutdownErr := errors.New("shutdown failed")
	closeErr := errors.New("close failed")
	firewallErr := errors.New("firewall cleanup failed")
	restoreErr := errors.New("restore failed")
	var events []string
	server.shutdown = func(context.Context) error {
		events = append(events, "drain")
		return shutdownErr
	}
	server.close = func() error {
		events = append(events, "HTTP")
		return closeErr
	}
	lease := &fakeFirewallLease{close: func(context.Context) error {
		assertFileOpen(t, file)
		events = append(events, "firewall")
		return firewallErr
	}}
	a.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) { return lease, nil }
	quit := make(chan struct{})
	close(quit)
	a.startShutdownListener = func() (<-chan struct{}, func() error, error) {
		return quit, func() error {
			assertFileClosed(t, file)
			events = append(events, "terminal")
			return restoreErr
		}, nil
	}
	err := a.Run(context.Background(), Request{Paths: []string{path}, Lifetime: time.Hour})
	for _, want := range []error{shutdownErr, closeErr, firewallErr, restoreErr} {
		if !errors.Is(err, want) {
			t.Errorf("Run() error = %v, missing %v", err, want)
		}
	}
	if want := []string{"drain", "HTTP", "firewall", "terminal"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("cleanup = %v, want %v", events, want)
	}
	if lease.closeCalls != 1 {
		t.Fatalf("firewall closed %d times, want once", lease.closeCalls)
	}
}

// Cancellation must restore the terminal even when it arrives after q, expiry,
// or server failure has started cleanup.
func TestApplicationRestoresTerminalWhileTextCleanupBlocked(t *testing.T) {
	for _, reason := range []string{"signal", "q", "expiration", "server failure"} {
		t.Run(reason, func(t *testing.T) {
			a, server, _, _ := newTestApplication(t)
			a.stderr = io.Discard
			a.openReceiveStore = func(string) (receiveStore, error) { return nil, nil }
			sinkStarted := make(chan context.Context, 1)
			releaseSink := make(chan struct{})
			release := sync.OnceFunc(func() { close(releaseSink) })
			defer release()
			a.newClipboardSink = func(string) (receive.TextSink, error) {
				return textSinkFunc(func(ctx context.Context, _ share.Text) error {
					sinkStarted <- ctx
					<-releaseSink // Deliberately ignores cancellation, like a blocked stdout.
					return nil
				}), nil
			}
			var processorCtx context.Context
			submitDone := make(chan error, 1)
			a.newReceiveServer = func(_ *session.Session, _ receiveStore, submitter textSubmitter) sessionServer {
				text, err := share.NewText([]byte("blocked text"))
				if err != nil {
					panic(err)
				}
				go func() { submitDone <- submitter.Submit(context.Background(), text) }()
				processorCtx = <-sinkStarted
				return server
			}
			ready := make(chan struct{})
			restored := make(chan struct{})
			restoreErr := errors.New("restore failed")
			serverErr := errors.New("server failed")
			restoreCalls := 0
			a.startShutdownListener = func() (<-chan struct{}, func() error, error) {
				quit := make(chan struct{})
				if reason == "q" {
					close(quit)
				}
				if reason == "server failure" {
					server.done <- serverErr
				}
				close(ready)
				return quit, func() error {
					restoreCalls++
					close(restored)
					return restoreErr
				}, nil
			}
			firewallClosed := make(chan struct{})
			a.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
				return &fakeFirewallLease{close: func(ctx context.Context) error {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					close(firewallClosed)
					return nil
				}}, nil
			}
			lifetime := time.Hour
			if reason == "expiration" {
				lifetime = time.Nanosecond
			}
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			done := make(chan error, 1)
			go func() {
				done <- a.Run(ctx, Request{Operation: OperationReceive, Clipboard: "xclip", Lifetime: lifetime})
			}()
			awaitLifecycle(t, ready)
			if reason != "signal" {
				awaitLifecycle(t, firewallClosed)
				if reason != "q" {
					// Expiration and failure cancel text rather than draining it.
					awaitLifecycle(t, processorCtx.Done())
				}
			}
			select {
			case <-restored:
				t.Fatal("terminal restored before text cleanup without a signal")
			default:
			}
			cause := errors.New("signal")
			cancel(cause)
			awaitLifecycle(t, restored)
			awaitLifecycle(t, processorCtx.Done())
			select {
			case err := <-done:
				t.Fatalf("Run() returned before sink finished: %v", err)
			default:
			}
			release()
			select {
			case err := <-done:
				if !errors.Is(err, restoreErr) {
					t.Fatalf("Run() error = %v, missing restoration error", err)
				}
				var want error
				switch reason {
				case "signal", "q":
					want = cause
				case "server failure":
					want = serverErr
				}
				if want != nil && !errors.Is(err, want) {
					t.Fatalf("Run() error = %v, missing %v", err, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Run() did not finish after releasing the sink")
			}
			<-submitDone
			if restoreCalls != 1 {
				t.Fatalf("restore calls = %d, want 1", restoreCalls)
			}
			wantDrains := 0
			if reason == "q" || reason == "expiration" {
				wantDrains = 1
			}
			if server.shutdownCalls != wantDrains || server.closeCalls != 1-wantDrains {
				t.Fatalf("HTTP shutdown=%d close=%d", server.shutdownCalls, server.closeCalls)
			}
		})
	}
}

func TestApplicationRestoresTerminalDuringHTTPDrain(t *testing.T) {
	for _, reason := range []string{"q", "expiration"} {
		t.Run(reason, func(t *testing.T) {
			a, server, _, path := newTestApplication(t)
			restored := make(chan struct{})
			a.startShutdownListener = func() (<-chan struct{}, func() error, error) {
				quit := make(chan struct{})
				if reason == "q" {
					close(quit)
				}
				return quit, func() error { close(restored); return nil }, nil
			}
			drainStarted := make(chan context.Context, 1)
			releaseDrain := make(chan struct{})
			release := sync.OnceFunc(func() { close(releaseDrain) })
			defer release()
			server.shutdown = func(ctx context.Context) error {
				drainStarted <- ctx
				select {
				case <-ctx.Done():
					return context.Cause(ctx)
				case <-releaseDrain:
					return nil
				}
			}
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			lifetime := time.Hour
			if reason == "expiration" {
				lifetime = time.Nanosecond
			}
			done := make(chan error, 1)
			go func() { done <- a.Run(ctx, Request{Paths: []string{path}, Lifetime: lifetime}) }()
			var drainCtx context.Context
			select {
			case drainCtx = <-drainStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("HTTP drain did not start")
			}
			cause := errors.New("signal during drain")
			cancel(cause)
			awaitLifecycle(t, restored)
			if reason == "q" {
				awaitLifecycle(t, drainCtx.Done())
			} else {
				if drainCtx.Err() != nil {
					t.Fatal("signal interrupted expiration drain")
				}
				select {
				case err := <-done:
					t.Fatalf("Run() returned during expiration drain: %v", err)
				default:
				}
				release()
			}
			select {
			case err := <-done:
				if reason == "q" && !errors.Is(err, cause) {
					t.Fatalf("Run() error = %v, want cancellation", err)
				}
				if reason == "expiration" && err != nil {
					t.Fatalf("Run() error = %v, want successful expiration", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Run() did not finish")
			}
		})
	}
}

func awaitLifecycle(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lifecycle event")
	}
}

func assertFileOpen(t *testing.T, file *share.File) {
	t.Helper()
	if _, err := file.Reader().Read(make([]byte, 1)); err != nil {
		t.Errorf("shared file closed before HTTP/firewall cleanup: %v", err)
	}
}

func assertFileClosed(t *testing.T, file *share.File) {
	t.Helper()
	if _, err := file.Reader().Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
		t.Errorf("shared file not released: %v", err)
	}
}
