package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/canta-9142/qshare/internal/share"
)

var ErrUnsupportedBackend = errors.New("unsupported clipboard backend")

type Sink struct {
	executable string
	arguments  []string
}

func NewSink(backend string) (*Sink, error) {
	switch backend {
	case "wl-copy":
		return &Sink{executable: "wl-copy"}, nil
	case "xclip":
		return &Sink{
			executable: "xclip",
			arguments:  []string{"-selection", "clipboard"},
		}, nil
	case "xsel":
		return &Sink{
			executable: "xsel",
			arguments:  []string{"--clipboard", "--input"},
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedBackend, backend)
	}
}

func (s *Sink) WriteText(ctx context.Context, text share.Text) error {
	command := exec.CommandContext(ctx, s.executable, s.arguments...)
	command.Stdin = bytes.NewReader(text.Bytes())
	if err := command.Run(); err != nil {
		return fmt.Errorf("run clipboard backend %s: %w", s.executable, err)
	}
	return nil
}
