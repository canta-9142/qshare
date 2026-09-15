package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/platform/firewall"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

func TestApplicationCleanupContinuesAfterErrors(t *testing.T) {
	a, listener, _, path := newTestApplication(t)
	var file *share.File
	a.openCollection = func(paths []string) (*share.Collection, error) {
		files, err := share.OpenCollection(paths)
		if err == nil {
			file = files.Resources()[0].File()
		}
		return files, err
	}
	closeErr := errors.New("listener close failed")
	firewallErr := errors.New("firewall cleanup failed")
	restoreErr := errors.New("restore failed")
	listener.closeErr = closeErr
	var events []string
	listener.onClose = func() {
		assertFileOpen(t, file)
		events = append(events, "HTTP")
	}
	a.openFirewall = func(context.Context, firewall.Rule) (firewallLease, error) {
		return firewallLeaseFunc(func(context.Context) error {
			assertFileOpen(t, file)
			events = append(events, "firewall")
			return firewallErr
		}), nil
	}
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
	for _, want := range []error{closeErr, firewallErr, restoreErr} {
		if !errors.Is(err, want) {
			t.Errorf("Run() error = %v, missing %v", err, want)
		}
	}
	if want := []string{"HTTP", "firewall", "terminal"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("cleanup = %v, want %v", events, want)
	}
}

func TestSessionQuitDrainsHTTP(t *testing.T) {
	request := newBlockedHTTPRequest(t)
	restored := make(chan struct{})
	request.run.watchTerminal(t.Context(), func() error { close(restored); return nil })
	quit := make(chan struct{})
	done := startSessionRun(request.run, t.Context(), quit)

	close(quit)
	awaitLifecycle(t, request.listener.closed)
	assertPending(t, done)
	assertNotClosed(t, restored, "terminal restored before request drained")

	request.release()
	if err := awaitRun(t, request.done); err != nil {
		t.Fatalf("request did not drain: %v", err)
	}
	if err := awaitRun(t, done); err != nil {
		t.Fatalf("q drain error = %v", err)
	}
	awaitLifecycle(t, restored)
}

func TestSessionSignalInterruptsQuitHTTPDrain(t *testing.T) {
	request := newBlockedHTTPRequest(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	restored := make(chan struct{})
	request.run.watchTerminal(ctx, func() error { close(restored); return nil })
	quit := make(chan struct{})
	done := startSessionRun(request.run, ctx, quit)

	close(quit)
	awaitLifecycle(t, request.listener.closed)
	cause := errors.New("signal during q drain")
	cancel(cause)
	awaitLifecycle(t, restored)
	awaitLifecycle(t, request.ctx.Done())
	if err := awaitRun(t, done); !errors.Is(err, cause) {
		t.Fatalf("error = %v, want signal", err)
	}
	request.release()
	_ = awaitRun(t, request.done)
}

func TestSessionSignalPreservesExpirationHTTPDrain(t *testing.T) {
	request := newBlockedHTTPRequest(t)
	request.run.session, _ = session.NewReceive(time.Nanosecond)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	restored := make(chan struct{})
	request.run.watchTerminal(ctx, func() error { close(restored); return nil })
	done := startSessionRun(request.run, ctx, nil)

	awaitLifecycle(t, request.listener.closed)
	cancel(errors.New("signal during expiration drain"))
	awaitLifecycle(t, restored)
	assertPending(t, done)
	assertNotClosed(t, request.ctx.Done(), "signal interrupted expiration drain")

	request.release()
	if err := awaitRun(t, request.done); err != nil {
		t.Fatalf("request did not drain: %v", err)
	}
	if err := awaitRun(t, done); err != nil {
		t.Fatalf("expiration error = %v", err)
	}
}

func TestSessionSignalClosesActiveHTTP(t *testing.T) {
	request := newBlockedHTTPRequest(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	restored := make(chan struct{})
	request.run.watchTerminal(ctx, func() error { close(restored); return nil })
	done := startSessionRun(request.run, ctx, nil)

	cause := errors.New("signal")
	cancel(cause)
	awaitLifecycle(t, request.listener.closed)
	awaitLifecycle(t, restored)
	awaitLifecycle(t, request.ctx.Done())
	if err := awaitRun(t, done); !errors.Is(err, cause) {
		t.Fatalf("error = %v, want signal", err)
	}
	request.release()
	_ = awaitRun(t, request.done)
}

func TestShutdownSessionServerTimeout(t *testing.T) {
	request := newBlockedHTTPRequest(t)
	if err := shutdownSessionServer(t.Context(), request.run.server, 0); err != nil {
		t.Fatalf("timeout should force close successfully: %v", err)
	}
	awaitLifecycle(t, request.ctx.Done())
	awaitLifecycle(t, request.listener.closed)
	if err := awaitRun(t, request.run.serverDone); err != nil {
		t.Fatal(err)
	}
	request.release()
	_ = awaitRun(t, request.done)
}

func TestSessionQuitDrainsAcceptedText(t *testing.T) {
	run, listener := newLifecycleRun(t, http.NotFoundHandler())
	processorCtx, submissionDone, release := blockTextProcessing(t, run)
	restored := make(chan struct{})
	run.watchTerminal(t.Context(), func() error { close(restored); return nil })
	quit := make(chan struct{})
	done := startSessionRun(run, t.Context(), quit)

	close(quit)
	awaitLifecycle(t, listener.closed)
	assertPending(t, done)
	assertNotClosed(t, processorCtx.Done(), "q cancelled accepted text")
	assertNotClosed(t, restored, "terminal restored before text drained")

	release()
	if err := awaitRun(t, submissionDone); err != nil {
		t.Fatalf("accepted text not drained: %v", err)
	}
	if err := awaitRun(t, done); err != nil {
		t.Fatalf("q drain error = %v", err)
	}
	awaitLifecycle(t, restored)
}

func TestSessionSignalInterruptsTextDrain(t *testing.T) {
	run, _ := newLifecycleRun(t, http.NotFoundHandler())
	processorCtx, submissionDone, release := blockTextProcessing(t, run)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	restored := make(chan struct{})
	run.watchTerminal(ctx, func() error { close(restored); return nil })
	firewallClosed := make(chan struct{})
	run.lease = firewallLeaseFunc(func(context.Context) error { close(firewallClosed); return nil })
	quit := make(chan struct{})
	done := startSessionRun(run, ctx, quit)

	close(quit)
	awaitLifecycle(t, firewallClosed) // HTTP has finished; cleanup can now drain text.
	assertNotClosed(t, restored, "terminal restored before text drained")
	cause := errors.New("signal during text drain")
	cancel(cause)
	awaitLifecycle(t, restored)
	awaitLifecycle(t, processorCtx.Done())
	assertPending(t, done) // The output write still blocks after cancellation.

	release()
	if err := awaitRun(t, done); !errors.Is(err, cause) {
		t.Fatalf("error = %v, want signal", err)
	}
	_ = awaitRun(t, submissionDone)
}

func TestSessionExpirationCancelsTextAndAllowsEarlyRestoration(t *testing.T) {
	run, _ := newLifecycleRun(t, http.NotFoundHandler())
	processorCtx, submissionDone, release := blockTextProcessing(t, run)
	run.session, _ = session.NewReceive(time.Nanosecond)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	restored := make(chan struct{})
	run.watchTerminal(ctx, func() error { close(restored); return nil })
	done := startSessionRun(run, ctx, nil)

	awaitLifecycle(t, processorCtx.Done())
	assertNotClosed(t, restored, "terminal restored before text cleanup without a signal")
	cancel(errors.New("signal after expiration"))
	awaitLifecycle(t, restored)
	assertPending(t, done)

	release()
	if err := awaitRun(t, done); err != nil {
		t.Fatalf("expiration error = %v", err)
	}
	_ = awaitRun(t, submissionDone)
}

func TestSessionSignalRestoresTerminalWhileTextBlocked(t *testing.T) {
	run, _ := newLifecycleRun(t, http.NotFoundHandler())
	processorCtx, submissionDone, release := blockTextProcessing(t, run)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	restored := make(chan struct{})
	restoreErr := errors.New("restore failed")
	run.watchTerminal(ctx, func() error { close(restored); return restoreErr })
	done := startSessionRun(run, ctx, nil)

	cause := errors.New("signal")
	cancel(cause)
	awaitLifecycle(t, restored)
	awaitLifecycle(t, processorCtx.Done())
	assertPending(t, done)

	release()
	if err := awaitRun(t, done); !errors.Is(err, cause) || !errors.Is(err, restoreErr) {
		t.Fatalf("error = %v, want signal and restoration errors", err)
	}
	_ = awaitRun(t, submissionDone)
}

func TestSessionServerFailureCancelsTextAndAllowsEarlyRestoration(t *testing.T) {
	run, listener := newLifecycleRun(t, http.NotFoundHandler())
	processorCtx, submissionDone, release := blockTextProcessing(t, run)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	restored := make(chan struct{})
	run.watchTerminal(ctx, func() error { close(restored); return nil })
	done := startSessionRun(run, ctx, nil)

	listener.Listener.Close()
	awaitLifecycle(t, processorCtx.Done())
	assertNotClosed(t, restored, "terminal restored before text cleanup without a signal")
	cancel(errors.New("signal after server failure"))
	awaitLifecycle(t, restored)
	assertPending(t, done)

	release()
	if err := awaitRun(t, done); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("error = %v, want listener failure", err)
	}
	_ = awaitRun(t, submissionDone)
}

// Setup helpers arrange blocked work; each test controls its own exit sequence.
type blockedHTTPRequest struct {
	run      *sessionRun
	listener *testListener
	ctx      context.Context
	done     <-chan error
	release  func()
}

func newBlockedHTTPRequest(t *testing.T) *blockedHTTPRequest {
	t.Helper()
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	run, listener := newLifecycleRun(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.Context()
		<-release
		fmt.Fprint(w, "finished")
	}))
	t.Cleanup(unblock)
	requestDone := startTestRequest(listener)
	select {
	case ctx := <-started:
		return &blockedHTTPRequest{run, listener, ctx, requestDone, unblock}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP request did not start")
	}
	return nil
}

func blockTextProcessing(t *testing.T, run *sessionRun) (context.Context, <-chan error, func()) {
	t.Helper()
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	run.textProcessor = receive.NewTextProcessor(textSinkFunc(func(ctx context.Context, _ share.Text) error {
		started <- ctx
		<-release
		return nil
	}), receive.TextQueueCapacity)
	t.Cleanup(func() {
		unblock()
		run.textProcessor.Close()
	})
	text, _ := share.NewText([]byte("accepted text"))
	done := make(chan error, 1)
	go func() { done <- run.textProcessor.Submit(t.Context(), text) }()
	select {
	case ctx := <-started:
		return ctx, done, unblock
	case <-time.After(2 * time.Second):
		t.Fatal("text processing did not start")
	}
	return nil, done, unblock
}

func startSessionRun(run *sessionRun, ctx context.Context, quit <-chan struct{}) <-chan error {
	done := make(chan error, 1)
	go func() {
		end, err := run.wait(ctx, quit)
		done <- run.finish(ctx, end, err)
	}()
	return done
}

func assertNotClosed(t *testing.T, done <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-done:
		t.Error(message)
	default:
	}
}

func newLifecycleRun(t *testing.T, h http.Handler) (*sessionRun, *testListener) {
	t.Helper()
	ln := newTestListener(t)
	sess, err := session.NewReceive(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	run := &sessionRun{session: sess, server: server.NewHTTPServer(h), listener: ln}
	run.serve()
	t.Cleanup(func() { run.server.Close() })
	return run, ln
}

func startTestRequest(listener net.Listener) <-chan error {
	done := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_, err = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		done <- err
	}()
	return done
}

func awaitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run")
	}
	return nil
}

func assertPending(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("run returned before cleanup completed: %v", err)
	default:
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
