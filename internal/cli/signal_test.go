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

func TestExitCodeForError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "SIGINT",
			err:  &terminationSignal{signal: os.Interrupt},
			want: 130,
		},
		{
			name: "SIGTERM",
			err:  &terminationSignal{signal: syscall.SIGTERM},
			want: 143,
		},
		{
			name: "signal joined with cleanup error",
			err: errors.Join(
				&terminationSignal{signal: os.Interrupt},
				errors.New("cleanup failed"),
			),
			want: 1,
		},
		{
			name: "runtime error",
			err:  errors.New("server failed"),
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeForError(tt.err); got != tt.want {
				t.Fatalf("exitCodeForError() = %d, want %d", got, tt.want)
			}
		})
	}
}
