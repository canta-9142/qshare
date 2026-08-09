package session

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

const tokenBytes = 32

type Token [tokenBytes]byte

func NewToken() (Token, error) {
	var token Token

	if _, err := rand.Read(token[:]); err != nil {
		return Token{}, fmt.Errorf("generate session token: %w", err)
	}

	return token, nil
}

func (t Token) Equal(other Token) bool {
	return subtle.ConstantTimeCompare(
		t[:],
		other[:],
	) == 1
}

func (t Token) String() string {
	return base64.RawURLEncoding.EncodeToString(t[:])
}

func ParseToken(s string) (Token, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Token{}, fmt.Errorf("decode session token: %w", err)
	}

	if len(raw) != len(Token{}) {
		return Token{}, fmt.Errorf("invalid session token length: got %d, want %d", len(raw), len(Token{}))
	}

	var token Token
	copy(token[:], raw)

	return token, nil
}
