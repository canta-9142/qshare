package qr

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestValidateMatrix(t *testing.T) {
	tests := []struct {
		name   string
		bitmap [][]bool
		want   bool
	}{
		{
			name: "square matrix",
			bitmap: [][]bool{
				{true, false},
				{false, true},
			},
			want: true,
		},
		{
			name:   "nil matrix",
			bitmap: nil,
			want:   false,
		},
		{
			name:   "empty row",
			bitmap: [][]bool{{}},
			want:   false,
		},
		{
			name: "rectangular matrix",
			bitmap: [][]bool{
				{true, false},
			},
			want: false,
		},
		{
			name: "uneven rows",
			bitmap: [][]bool{
				{true, false},
				{true},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateMatrix(tt.bitmap); got != tt.want {
				t.Fatalf("validateMatrix() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAddMatrixMargin(t *testing.T) {
	bitmap := [][]bool{
		{true, false},
		{false, true},
	}
	want := [][]bool{
		{false, false, false, false},
		{false, true, false, false},
		{false, false, true, false},
		{false, false, false, false},
	}

	got := addMatrixMargin(bitmap, 1)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("addMatrixMargin() = %v, want %v", got, want)
	}
}

func TestRenderBitmap(t *testing.T) {
	bitmap := [][]bool{
		{true, true, false, false},
		{true, false, true, false},
		{false, false, false, false},
		{false, false, false, false},
	}

	var dst strings.Builder
	if err := renderBitmap(&dst, bitmap); err != nil {
		t.Fatalf("renderBitmap() error = %v", err)
	}

	want := "█▀▄ \n    \n"
	if got := dst.String(); got != want {
		t.Fatalf("renderBitmap() = %q, want %q", got, want)
	}
}

func TestRenderBitmapOddHeight(t *testing.T) {
	bitmap := [][]bool{{true}}

	var dst strings.Builder
	if err := renderBitmap(&dst, bitmap); err != nil {
		t.Fatalf("renderBitmap() error = %v", err)
	}

	want := "▀\n"
	if got := dst.String(); got != want {
		t.Fatalf("renderBitmap() = %q, want %q", got, want)
	}
}

func TestRender(t *testing.T) {
	t.Run("writes QR code", func(t *testing.T) {
		var dst bytes.Buffer

		if err := Render(&dst, "http://192.0.2.1:8080/d/token"); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if dst.Len() == 0 {
			t.Fatal("Render() wrote no output")
		}
	})

	t.Run("returns writer error", func(t *testing.T) {
		writeErr := errors.New("write failed")
		dst := errorWriter{err: writeErr}

		err := Render(dst, "http://192.0.2.1:8080/d/token")
		if !errors.Is(err, writeErr) {
			t.Fatalf("Render() error = %v, want writer error", err)
		}
	})
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
