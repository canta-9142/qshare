package clipboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/canta-9142/qshare/internal/share"
)

func TestSinkInvokesFixedBackendWithTextOnStdin(t *testing.T) {
	tests := []struct {
		backend  string
		wantArgs []string
	}{
		{backend: "wl-copy"},
		{backend: "xclip", wantArgs: []string{"-selection", "clipboard"}},
		{backend: "xsel", wantArgs: []string{"--clipboard", "--input"}},
	}

	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "input")
			argsPath := filepath.Join(dir, "args")
			writeBackendScript(t, dir, tt.backend, `#!/bin/sh
printf '%s\n' "$@" > "$QSHARE_TEST_ARGS"
cat > "$QSHARE_TEST_INPUT"
`)
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("QSHARE_TEST_INPUT", inputPath)
			t.Setenv("QSHARE_TEST_ARGS", argsPath)

			sink, err := NewSink(tt.backend)
			if err != nil {
				t.Fatalf("NewSink() error = %v", err)
			}
			value := `text with $HOME; $(touch should-not-run)`
			if err := sink.WriteText(context.Background(), mustText(t, value)); err != nil {
				t.Fatalf("WriteText() error = %v", err)
			}

			input, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(input) != value {
				t.Errorf("stdin = %q, want %q", input, value)
			}
			args, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			var gotArgs []string
			if value := strings.TrimSuffix(string(args), "\n"); value != "" {
				gotArgs = strings.Split(value, "\n")
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("arguments = %q, want %q", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestSinkReportsBackendFailure(t *testing.T) {
	dir := t.TempDir()
	writeBackendScript(t, dir, "wl-copy", "#!/bin/sh\nexit 7\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	sink, err := NewSink("wl-copy")
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteText(context.Background(), mustText(t, "value")); err == nil {
		t.Fatal("WriteText() error = nil, want backend failure")
	}
}

func TestNewSinkRejectsUnsupportedBackend(t *testing.T) {
	for _, backend := range []string{"", "auto", "sh", "wl-copy --help"} {
		if _, err := NewSink(backend); !errors.Is(err, ErrUnsupportedBackend) {
			t.Errorf("NewSink(%q) error = %v, want ErrUnsupportedBackend", backend, err)
		}
	}
}

func writeBackendScript(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustText(t *testing.T, value string) share.Text {
	t.Helper()
	text, err := share.NewText([]byte(value))
	if err != nil {
		t.Fatal(err)
	}
	return text
}
