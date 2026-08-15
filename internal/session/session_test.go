package session

import (
	"testing"
	"time"
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
			if _, err := NewSend(nil, lifetime); err == nil {
				t.Fatal("NewSend() error = nil, want error")
			}
			if _, err := NewReceive(lifetime); err == nil {
				t.Fatal("NewReceive() error = nil, want error")
			}
		})
	}
}

func TestNewReceiveCreatesSessionWithoutResource(t *testing.T) {
	session, err := NewReceive(time.Minute)
	if err != nil {
		t.Fatalf("NewReceive() error = %v", err)
	}
	if session.Resource() != nil {
		t.Fatal("Resource() is not nil for receive session")
	}
	if !session.Authorize(session.Token(), time.Now()) {
		t.Fatal("receive session does not authorize its token")
	}
}
