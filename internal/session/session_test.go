package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

func TestSessionAuthorize(t *testing.T) {
	token := testToken(1)
	expiresAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	session := &Session{
		token:     token,
		expiresAt: expiresAt,
	}

	tests := []struct {
		name      string
		candidate Token
		now       time.Time
		want      bool
	}{
		{
			name:      "valid before expiration",
			candidate: token,
			now:       expiresAt.Add(-time.Nanosecond),
			want:      true,
		},
		{
			name:      "wrong token",
			candidate: testToken(2),
			now:       expiresAt.Add(-time.Minute),
			want:      false,
		},
		{
			name:      "exact expiration boundary",
			candidate: token,
			now:       expiresAt,
			want:      false,
		},
		{
			name:      "after expiration",
			candidate: token,
			now:       expiresAt.Add(time.Nanosecond),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := session.Authorize(tt.candidate, tt.now); got != tt.want {
				t.Fatalf("Authorize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionTokensAreSeparated(t *testing.T) {
	expiresAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	first := &Session{token: testToken(1), expiresAt: expiresAt}
	second := &Session{token: testToken(2), expiresAt: expiresAt}
	now := expiresAt.Add(-time.Minute)

	if first.Authorize(second.Token(), now) {
		t.Fatal("first session authorized second session token")
	}
	if second.Authorize(first.Token(), now) {
		t.Fatal("second session authorized first session token")
	}
}

func TestNewRejectsNonPositiveLifetime(t *testing.T) {
	for _, lifetime := range []time.Duration{0, -time.Nanosecond} {
		t.Run(lifetime.String(), func(t *testing.T) {
			if _, err := NewSendFiles(nil, lifetime); err == nil {
				t.Fatal("NewSendFiles() error = nil, want error")
			}
			if _, err := NewSendText(share.Text{}, lifetime); err == nil {
				t.Fatal("NewSendText() error = nil, want error")
			}
			if _, err := NewReceive(lifetime); err == nil {
				t.Fatal("NewReceive() error = nil, want error")
			}
		})
	}
}

func TestNewSendTextStoresText(t *testing.T) {
	text, err := share.NewText([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSendText(text, time.Minute)
	if err != nil {
		t.Fatalf("NewSendText() error = %v", err)
	}
	got, ok := session.Text()
	if !ok || got.String() != "hello" {
		t.Fatalf("Text() = %q, %v; want hello, true", got.String(), ok)
	}
	if session.Resources() != nil {
		t.Fatal("Resources() is not nil for text send session")
	}
}

func TestNewReceiveCreatesSessionWithoutResource(t *testing.T) {
	session, err := NewReceive(time.Minute)
	if err != nil {
		t.Fatalf("NewReceive() error = %v", err)
	}
	if session.Resources() != nil {
		t.Fatal("Resources() is not nil for receive session")
	}
	if !session.Authorize(session.Token(), time.Now()) {
		t.Fatal("receive session does not authorize its token")
	}
}

func TestSendSessionResolve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	resources, err := share.OpenCollection([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resources.Close() })
	sess, err := NewSendFiles(resources, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	id := resources.Resources()[0].ID()
	if got, ok := sess.Resolve(sess.Token(), id, time.Now()); !ok || got.Name() != "shared.txt" {
		t.Fatal("Resolve() rejected valid resource")
	}
	if _, ok := sess.Resolve(sess.Token(), "unknown", time.Now()); ok {
		t.Fatal("Resolve() accepted unknown ID")
	}
	wrong := sess.Token()
	wrong[0] ^= 0xff
	if _, ok := sess.Resolve(wrong, id, time.Now()); ok {
		t.Fatal("Resolve() accepted invalid token")
	}
	if _, ok := sess.Resolve(sess.Token(), id, sess.ExpiresAt()); ok {
		t.Fatal("Resolve() accepted expired session")
	}
}

func TestSendSessionRejectsResourceFromAnotherSession(t *testing.T) {
	paths := make([]string, 2)
	for i := range paths {
		paths[i] = filepath.Join(t.TempDir(), fmt.Sprintf("%d.txt", i))
		if err := os.WriteFile(paths[i], []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := share.OpenCollection(paths[:1])
	defer first.Close()
	second, _ := share.OpenCollection(paths[1:])
	defer second.Close()
	sess, _ := NewSendFiles(first, time.Hour)
	if _, ok := sess.Resolve(sess.Token(), second.Resources()[0].ID(), time.Now()); ok {
		t.Fatal("Resolve() accepted another session's resource ID")
	}
}
