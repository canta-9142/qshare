//go:build linux

package firewall

import (
	"context"
	"errors"
	"testing"
)

type fakeBackend struct {
	lease   Lease
	handled bool
	err     error
	calls   int
}

func (b *fakeBackend) tryOpen(context.Context, Rule) (Lease, bool, error) {
	b.calls++
	return b.lease, b.handled, b.err
}

func TestManagerFallsBackToNextBackend(t *testing.T) {
	first := &fakeBackend{}
	want := noopLease{}
	second := &fakeBackend{lease: want, handled: true}
	third := &fakeBackend{lease: noopLease{}, handled: true}

	got, err := (&manager{backends: []backend{first, second, third}}).open(context.Background(), testRule())
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	if got != want {
		t.Fatalf("open() lease = %#v, want %#v", got, want)
	}
	if first.calls != 1 || second.calls != 1 || third.calls != 0 {
		t.Fatalf("backend calls = %d, %d, %d; want 1, 1, 0", first.calls, second.calls, third.calls)
	}
}

func TestManagerStopsAfterBackendFailure(t *testing.T) {
	want := errors.New("backend failed")
	first := &fakeBackend{handled: true, err: want}
	second := &fakeBackend{handled: true, lease: noopLease{}}

	_, err := (&manager{backends: []backend{first, second}}).open(context.Background(), testRule())
	if !errors.Is(err, want) {
		t.Fatalf("open() error = %v, want %v", err, want)
	}
	if second.calls != 0 {
		t.Fatalf("second backend calls = %d, want 0", second.calls)
	}
}
