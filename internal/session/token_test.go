package session

import (
	"encoding/base64"
	"testing"
)

func TestTokenStringAndParseRoundTrip(t *testing.T) {
	want := testToken(1)

	encoded := want.String()
	if len(encoded) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		t.Fatalf("encoded token length = %d, want %d", len(encoded), base64.RawURLEncoding.EncodedLen(tokenBytes))
	}

	got, err := ParseToken(encoded)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if !got.Equal(want) {
		t.Fatal("parsed token does not equal original token")
	}
}

func TestParseTokenRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "invalid encoding", input: "%%%"},
		{name: "empty", input: ""},
		{name: "too short", input: base64.RawURLEncoding.EncodeToString(make([]byte, tokenBytes-1))},
		{name: "too long", input: base64.RawURLEncoding.EncodeToString(make([]byte, tokenBytes+1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseToken(tt.input); err == nil {
				t.Fatal("ParseToken() error = nil, want error")
			}
		})
	}
}

func TestTokenEqual(t *testing.T) {
	token := testToken(1)

	if !token.Equal(testToken(1)) {
		t.Fatal("Token.Equal() = false for equal tokens")
	}
	if token.Equal(testToken(2)) {
		t.Fatal("Token.Equal() = true for different tokens")
	}
}

func TestNewTokenProducesEncodableToken(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken() error = %v", err)
	}

	parsed, err := ParseToken(token.String())
	if err != nil {
		t.Fatalf("ParseToken(NewToken().String()) error = %v", err)
	}
	if !parsed.Equal(token) {
		t.Fatal("parsed generated token does not equal original token")
	}
}

func testToken(seed byte) Token {
	var token Token
	for i := range token {
		token[i] = seed + byte(i)
	}
	return token
}
