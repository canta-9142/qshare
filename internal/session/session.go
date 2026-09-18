package session

import (
	"fmt"
	"time"
)

// Session holds only the credentials and lifetime of one operation.
type Session struct {
	token     Token
	expiresAt time.Time
}

func New(lifetime time.Duration) (*Session, error) {
	if lifetime <= 0 {
		return nil, fmt.Errorf("lifetime must be positive: %s", lifetime)
	}

	token, err := NewToken()
	if err != nil {
		return nil, err
	}

	return &Session{
		token:     token,
		expiresAt: time.Now().Add(lifetime),
	}, nil
}

func (s *Session) Authorize(candidate Token, now time.Time) bool {
	if !now.Before(s.expiresAt) {
		return false
	}
	return s.token.Equal(candidate)
}

func (s *Session) Token() Token {
	return s.token
}

func (s *Session) ExpiresAt() time.Time {
	return s.expiresAt
}
