//go:build linux || darwin

package cli

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestSignalContextCapturesSIGTERM(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	select {
	case <-ctx.Done():
		var signalErr *terminationSignal
		if !errors.As(context.Cause(ctx), &signalErr) || signalErr.signal != syscall.SIGTERM {
			t.Fatalf("context cause = %v, want SIGTERM", context.Cause(ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("signal context was not cancelled")
	}
}
