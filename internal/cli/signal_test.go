package cli

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

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
