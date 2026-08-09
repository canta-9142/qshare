package session

import (
	"fmt"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

type Session struct {
	token     Token
	resource  *share.File
	expiresAt time.Time
}

func New(resource *share.File, lifetime time.Duration) (*Session, error) {
	if lifetime <= 0 {
		return nil, fmt.Errorf("lifetime must be positive: %s", lifetime)
	}

	token, err := NewToken()
	if err != nil {
		return nil, err
	}

	return &Session{
		token:     token,
		resource:  resource,
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

func (s *Session) Resource() *share.File {
	return s.resource
}

func (s *Session) ExpiresAt() time.Time {
	return s.expiresAt
}
