package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/canta-9142/qshare/internal/share"
)

var (
	ErrUnsupportedBackend = errors.New("unsupported clipboard backend")
	ErrBackendNotFound    = errors.New("clipboard backend executable not found")
)

var backendArguments = map[string][]string{
	"wl-copy": nil,
	"xclip":   {"-selection", "clipboard"},
	"xsel":    {"--clipboard", "--input"},
}

var automaticBackendOrder = []string{"wl-copy", "xclip", "xsel"}

type Sink struct {
	executable string
	arguments  []string
}

func NewSink(backend string) (*Sink, error) {
	if backend == "auto" {
		for _, candidate := range automaticBackendOrder {
			sink, err := sinkForBackend(candidate)
			if err == nil {
				return sink, nil
			}
			if !errors.Is(err, ErrBackendNotFound) {
				return nil, err
			}
		}
		return nil, fmt.Errorf("%w: tried %v", ErrBackendNotFound, automaticBackendOrder)
	}

	if _, ok := backendArguments[backend]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedBackend, backend)
	}
	return sinkForBackend(backend)
}

func sinkForBackend(backend string) (*Sink, error) {
	executable, err := exec.LookPath(backend)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrBackendNotFound, backend, err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve clipboard backend path %s: %w", backend, err)
	}

	arguments := append([]string(nil), backendArguments[backend]...)
	return &Sink{executable: executable, arguments: arguments}, nil
}

func (s *Sink) WriteText(ctx context.Context, text share.Text) error {
	command := exec.CommandContext(ctx, s.executable, s.arguments...)
	command.Stdin = bytes.NewReader(text.Bytes())
	if err := command.Run(); err != nil {
		return fmt.Errorf("run clipboard backend %s: %w", s.executable, err)
	}
	return nil
}
