package session

import (
	"fmt"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

type Session struct {
	token     Token
	resources *share.Collection
	text      *share.Text
	expiresAt time.Time
}

func NewSendFiles(resources *share.Collection, lifetime time.Duration) (*Session, error) {
	if resources == nil {
		return nil, fmt.Errorf("shared resource collection is required")
	}
	session, err := newSession(lifetime)
	if err != nil {
		return nil, err
	}

	session.resources = resources

	return session, nil
}

func NewSendText(text share.Text, lifetime time.Duration) (*Session, error) {
	session, err := newSession(lifetime)
	if err != nil {
		return nil, err
	}

	session.text = &text

	return session, nil
}

func NewReceive(lifetime time.Duration) (*Session, error) {
	session, err := newSession(lifetime)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func newSession(lifetime time.Duration) (*Session, error) {
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

func (s *Session) Resources() *share.Collection {
	return s.resources
}

// Resolve authorizes a token and resource ID against the same live session.
func (s *Session) Resolve(candidate Token, id share.ResourceID, now time.Time) (*share.File, bool) {
	if s.resources == nil || !s.Authorize(candidate, now) {
		return nil, false
	}
	return s.resources.Lookup(id)
}

func (s *Session) Text() (share.Text, bool) {
	if s.text == nil {
		return share.Text{}, false
	}
	return *s.text, true
}

func (s *Session) ExpiresAt() time.Time {
	return s.expiresAt
}
