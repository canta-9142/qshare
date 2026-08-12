package qr

import (
	"fmt"
	"io"
	"strings"

	"github.com/yeqown/go-qrcode/v2"
)

type terminalWriter struct {
	dst    io.Writer
	margin int
}

func (w *terminalWriter) Write(mat qrcode.Matrix) error {
	bitmap := mat.Bitmap()
	if ok := validateMatrix(bitmap); !ok {
		return fmt.Errorf("malformed QR bitmap")
	}

	bitmap = addMatrixMargin(bitmap, w.margin)

	return renderBitmap(w.dst, bitmap)
}

func (w *terminalWriter) Close() error {
	return nil
}

func Render(dst io.Writer, payload string) error {
	code, err := qrcode.NewWith(
		payload,
		qrcode.WithErrorCorrectionLevel(qrcode.ErrorCorrectionMedium),
	)
	if err != nil {
		return fmt.Errorf("encode QR payload: %w", err)
	}

	if err := code.Save(&terminalWriter{dst: dst, margin: 4}); err != nil {
		return fmt.Errorf("render QR code: %w", err)
	}

	return nil
}

func validateMatrix(bitmap [][]bool) bool {
	height := len(bitmap)
	if height == 0 {
		return false
	}

	width := len(bitmap[0])
	if width == 0 {
		return false
	}

	for _, row := range bitmap {
		if len(row) != width {
			return false
		}
	}

	if height != width {
		return false
	}

	return true
}

func addMatrixMargin(bitmap [][]bool, margin int) [][]bool {
	origL := len(bitmap)
	newL := origL + 2*margin

	result := make([][]bool, newL)
	for y := range result {
		result[y] = make([]bool, newL)
		for x := range result[y] {
			result[y][x] = false
		}
	}

	for y := 0; y < origL; y++ {
		for x := 0; x < origL; x++ {
			result[y+margin][x+margin] = bitmap[y][x]
		}
	}

	return result
}

func renderBitmap(dst io.Writer, bitmap [][]bool) error {
	l := len(bitmap)

	var buf strings.Builder

	for y := 0; y < l; y += 2 {
		for x := 0; x < l; x++ {
			top := bitmap[y][x]
			bottom := (y+1 < l) && bitmap[y+1][x]

			switch {
			case top && bottom:
				buf.WriteString("█")
			case top && !bottom:
				buf.WriteString("▀")
			case !top && bottom:
				buf.WriteString("▄")
			default:
				buf.WriteString(" ")
			}
		}
		buf.WriteByte('\n')
	}

	_, err := io.WriteString(dst, buf.String())
	return err
}
