package share

import (
	"bytes"
	"errors"
	"testing"
)

func TestNewTextPreservesValue(t *testing.T) {
	want := []byte("hello, 世界\n")
	text, err := NewText(want)
	if err != nil {
		t.Fatalf("NewText() error = %v", err)
	}

	if got := text.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("Bytes() = %q, want %q", got, want)
	}
	if got := text.String(); got != string(want) {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got := text.Size(); got != len(want) {
		t.Errorf("Size() = %d, want %d", got, len(want))
	}
}

func TestNewTextAcceptsSizeLimit(t *testing.T) {
	want := bytes.Repeat([]byte("x"), MaxTextSize)
	text, err := NewText(want)
	if err != nil {
		t.Fatalf("NewText() error = %v", err)
	}
	if !bytes.Equal(text.Bytes(), want) {
		t.Fatal("NewText() did not preserve text at the size limit")
	}
}

func TestNewTextRejectsValueOverSizeLimit(t *testing.T) {
	_, err := NewText(bytes.Repeat([]byte("x"), MaxTextSize+1))
	if !errors.Is(err, ErrTextTooLarge) {
		t.Fatalf("NewText() error = %v, want ErrTextTooLarge", err)
	}
}

func TestNewTextRejectsInvalidUTF8(t *testing.T) {
	_, err := NewText([]byte{0xff})
	if !errors.Is(err, ErrTextInvalidUTF8) {
		t.Fatalf("NewText() error = %v, want ErrTextInvalidUTF8", err)
	}
}

func TestTextDoesNotShareMutableBytes(t *testing.T) {
	input := []byte("original")
	text, err := NewText(input)
	if err != nil {
		t.Fatalf("NewText() error = %v", err)
	}

	input[0] = 'O'
	returned := text.Bytes()
	returned[1] = 'X'

	if got := text.String(); got != "original" {
		t.Fatalf("String() after byte mutation = %q, want %q", got, "original")
	}
}
