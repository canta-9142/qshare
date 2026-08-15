package share

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const MaxTextSize = 1 << 20

var (
	ErrTextTooLarge    = errors.New("text exceeds 1 MiB limit")
	ErrTextInvalidUTF8 = errors.New("text is not valid UTF-8")
)

// Text is a validated, immutable UTF-8 text value.
type Text struct {
	value string
}

// NewText validates value and preserves its bytes without normalization or
// other rewriting.
func NewText(value []byte) (Text, error) {
	if len(value) > MaxTextSize {
		return Text{}, fmt.Errorf("%w: got %d bytes", ErrTextTooLarge, len(value))
	}
	if !utf8.Valid(value) {
		return Text{}, ErrTextInvalidUTF8
	}

	return Text{value: string(value)}, nil
}

// String returns the original validated text.
func (t Text) String() string {
	return t.value
}

// Bytes returns a copy of the original validated bytes.
func (t Text) Bytes() []byte {
	return []byte(t.value)
}

// Size returns the size of the text in bytes.
func (t Text) Size() int {
	return len(t.value)
}
