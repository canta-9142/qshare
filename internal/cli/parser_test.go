package cli

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/app"
	"github.com/canta-9142/qshare/internal/share"
)

func TestMapArgumentsSelectsDirectoryMode(t *testing.T) {
	dir := t.TempDir()
	result, err := mapArguments(arguments{Files: []string{dir}, Expire: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if result.Request.Operation != app.OperationSendDirectory {
		t.Fatalf("Operation = %v", result.Request.Operation)
	}
	if _, err := mapArguments(arguments{Files: []string{dir, "file"}, Expire: time.Minute}); err == nil {
		t.Fatal("directory combined with file was accepted")
	}
}

func TestMapArgumentsSelectsPipedStdinText(t *testing.T) {
	want := "hello, 世界\n"
	result, err := mapArgumentsWithInput(arguments{Expire: time.Minute}, stdinInput{
		reader: strings.NewReader(want),
	})
	if err != nil {
		t.Fatalf("mapArgumentsWithInput() error = %v", err)
	}
	if result.Request.Operation != app.OperationSendText {
		t.Errorf("Operation = %v, want OperationSendText", result.Request.Operation)
	}
	if got := result.Request.Text.String(); got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

func TestMapArgumentsAcceptsStdinAtSizeLimit(t *testing.T) {
	want := strings.Repeat("x", 1<<20)
	result, err := mapArgumentsWithInput(arguments{Expire: time.Minute}, stdinInput{
		reader: strings.NewReader(want),
	})
	if err != nil {
		t.Fatalf("mapArgumentsWithInput() error = %v", err)
	}
	if got := result.Request.Text.String(); got != want {
		t.Fatal("stdin text at size limit was not preserved")
	}
}

func TestMapArgumentsRejectsInvalidPipedStdin(t *testing.T) {
	tests := []struct {
		name   string
		reader io.Reader
		want   error
	}{
		{
			name:   "over size limit",
			reader: strings.NewReader(strings.Repeat("x", 1<<20+1)),
			want:   share.ErrTextTooLarge,
		},
		{
			name:   "invalid UTF-8",
			reader: bytes.NewReader([]byte{0xff}),
			want:   share.ErrTextInvalidUTF8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mapArgumentsWithInput(arguments{Expire: time.Minute}, stdinInput{reader: tt.reader})
			if !errors.Is(err, tt.want) {
				t.Fatalf("mapArgumentsWithInput() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestMapArgumentsReadsAtMostSizeLimitPlusOne(t *testing.T) {
	reader := &countingReader{remaining: 1<<20 + 100}
	_, err := mapArgumentsWithInput(arguments{Expire: time.Minute}, stdinInput{reader: reader})
	if !errors.Is(err, share.ErrTextTooLarge) {
		t.Fatalf("mapArgumentsWithInput() error = %v, want ErrTextTooLarge", err)
	}
	if reader.read != 1<<20+1 {
		t.Fatalf("stdin bytes read = %d, want %d", reader.read, 1<<20+1)
	}
}

func TestMapArgumentsRejectsPipedStdinConflicts(t *testing.T) {
	text := "explicit"
	clipboard := "auto"
	tests := []struct {
		name string
		args arguments
	}{
		{name: "file", args: arguments{Expire: time.Minute, Files: []string{"file.txt"}}},
		{name: "text", args: arguments{Expire: time.Minute, Text: &text}},
		{name: "clipboard", args: arguments{Expire: time.Minute, Clipboard: &clipboard}},
		{name: "receive directory", args: arguments{Expire: time.Minute, ReceiveDir: "/tmp/received"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mapArgumentsWithInput(tt.args, stdinInput{reader: strings.NewReader("piped")}); err == nil {
				t.Fatal("mapArgumentsWithInput() error = nil, want conflict error")
			}
		})
	}
}

func TestMapArgumentsSelectsClipboardReceiveMode(t *testing.T) {
	backend := "backend-name"
	result, err := mapArguments(arguments{
		Expire:     time.Minute,
		ReceiveDir: "/tmp/received",
		Clipboard:  &backend,
	})
	if err != nil {
		t.Fatalf("mapArguments() error = %v", err)
	}
	if result.Request.Operation != app.OperationReceive {
		t.Errorf("Operation = %v, want OperationReceive", result.Request.Operation)
	}
	if result.Request.Clipboard != backend {
		t.Errorf("Clipboard = %q, want %q", result.Request.Clipboard, backend)
	}
}

func TestMapArgumentsRejectsClipboardConflicts(t *testing.T) {
	text := "text"
	tests := []struct {
		name      string
		configure func(*arguments, *string)
	}{
		{
			name: "with file",
			configure: func(args *arguments, _ *string) {
				args.Files = []string{"file.txt"}
			},
		},
		{
			name: "with explicit text",
			configure: func(args *arguments, _ *string) {
				args.Text = &text
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := "backend-name"
			args := arguments{Expire: time.Minute, Clipboard: &backend}
			if tt.configure != nil {
				tt.configure(&args, &backend)
			}
			if _, err := mapArguments(args); err == nil {
				t.Fatal("mapArguments() error = nil, want error")
			}
		})
	}
}

func TestMapArgumentsRejectsEmptyClipboardBackend(t *testing.T) {
	backend := ""
	if _, err := mapArguments(arguments{Expire: time.Minute, Clipboard: &backend}); err == nil {
		t.Fatal("mapArguments() error = nil, want error")
	}
}

func TestRunRejectsUnsupportedClipboardBackendAsUsageError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithInput(
		[]string{"--receive-dir", t.TempDir(), "--clipboard", "unsupported"},
		nil,
		true,
		&stdout,
		&stderr,
	)
	if code != 2 {
		t.Fatalf("runWithInput() = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unsupported clipboard backend") {
		t.Errorf("stderr = %q, want unsupported-backend diagnostic", stderr.String())
	}
}

func TestRunReportsMissingClipboardBackendAsRuntimeError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithInput(
		[]string{"--receive-dir", t.TempDir(), "--clipboard", "wl-copy"},
		nil,
		true,
		&stdout,
		&stderr,
	)
	if code != 1 {
		t.Fatalf("runWithInput() = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want missing-backend diagnostic", stderr.String())
	}
}

func TestPipedStdinConflictExitsWithUsageStatus(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithInput(
		[]string{"file.txt"},
		strings.NewReader("piped"),
		false,
		&stdout,
		&stderr,
	)
	if code != 2 {
		t.Fatalf("runWithInput() = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "piped stdin") {
		t.Errorf("stderr = %q, want conflict diagnostic", stderr.String())
	}
}

func TestParseHelpDoesNotReadPipedStdin(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := parseWithInput(
		[]string{"--help"},
		stdinInput{reader: errorReader{}},
		&stdout,
		&stderr,
	)
	if err != nil || !result.Exit || result.Code != 0 {
		t.Fatalf("parseWithInput() result=%+v error=%v, want help exit", result, err)
	}
}

type countingReader struct {
	remaining int
	read      int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.remaining)
	for i := range p[:n] {
		p[i] = 'x'
	}
	r.remaining -= n
	r.read += n
	return n, nil
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("unexpected read")
}

func TestParseMapsArguments(t *testing.T) {
	tests := []struct {
		name         string
		argv         []string
		wantPaths    string
		wantMode     app.NetworkMode
		wantLifetime time.Duration
	}{
		{
			name:         "defaults",
			argv:         []string{"example.txt"},
			wantPaths:    "example.txt",
			wantMode:     app.NetworkAuto,
			wantLifetime: 10 * time.Minute,
		},
		{
			name:         "LAN and expiration",
			argv:         []string{"--lan", "--expire", "30s", "example.txt"},
			wantPaths:    "example.txt",
			wantMode:     app.NetworkLAN,
			wantLifetime: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			result, err := parse(tt.argv, &stdout, &stderr)
			if err != nil {
				t.Fatalf("parse() error = %v", err)
			}
			if result.Exit {
				t.Fatalf("parse() Exit = true, code %d", result.Code)
			}
			if result.Request.Operation != app.OperationSendFile {
				t.Errorf("Operation = %v, want OperationSendFile", result.Request.Operation)
			}
			if result.Request.Paths[0] != tt.wantPaths {
				t.Errorf("Path = %q, want %q", result.Request.Paths[0], tt.wantPaths)
			}
			if result.Request.NetworkMode != tt.wantMode {
				t.Errorf("NetworkMode = %v, want %v", result.Request.NetworkMode, tt.wantMode)
			}
			if result.Request.Lifetime != tt.wantLifetime {
				t.Errorf("Lifetime = %v, want %v", result.Request.Lifetime, tt.wantLifetime)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestMapArgumentsSelectsTextSendMode(t *testing.T) {
	value := "hello, 世界"
	result, err := mapArguments(arguments{
		Expire: time.Minute,
		Text:   &value,
	})
	if err != nil {
		t.Fatalf("mapArguments() error = %v", err)
	}
	if result.Request.Operation != app.OperationSendText {
		t.Errorf("Operation = %v, want OperationSendText", result.Request.Operation)
	}
	if got := result.Request.Text.String(); got != value {
		t.Errorf("Text = %q, want %q", got, value)
	}
}

func TestParseTextOption(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := parse([]string{"--text", "hello"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if result.Exit {
		t.Fatalf("parse() Exit = true, code %d", result.Code)
	}
	if result.Request.Operation != app.OperationSendText || result.Request.Text.String() != "hello" {
		t.Fatalf("Request = %+v, want text send request", result.Request)
	}
}

func TestRunRejectsInvalidTextAsUsageError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := Run([]string{"--text", strings.Repeat("x", 1<<20+1)}, &stdout, &stderr); got != 2 {
		t.Fatalf("Run() = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "1 MiB") {
		t.Errorf("stderr = %q, want size-limit diagnostic", stderr.String())
	}
}

func TestMapArgumentsAcceptsEmptyExplicitText(t *testing.T) {
	value := ""
	result, err := mapArguments(arguments{Expire: time.Minute, Text: &value})
	if err != nil {
		t.Fatalf("mapArguments() error = %v", err)
	}
	if result.Request.Operation != app.OperationSendText {
		t.Errorf("Operation = %v, want OperationSendText", result.Request.Operation)
	}
}

func TestMapArgumentsRejectsInvalidTextRequests(t *testing.T) {
	valid := "text"
	invalidUTF8 := string([]byte{0xff})
	oversized := strings.Repeat("x", 1<<20+1)
	tests := []struct {
		name string
		args arguments
	}{
		{name: "text with file", args: arguments{Expire: time.Minute, Text: &valid, Files: []string{"file.txt"}}},
		{name: "text with receive directory", args: arguments{Expire: time.Minute, Text: &valid, ReceiveDir: "/tmp/received"}},
		{name: "invalid UTF-8", args: arguments{Expire: time.Minute, Text: &invalidUTF8}},
		{name: "oversized", args: arguments{Expire: time.Minute, Text: &oversized}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mapArguments(tt.args); err == nil {
				t.Fatal("mapArguments() error = nil, want error")
			}
		})
	}
}

func TestParseHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := parse([]string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if !result.Exit || result.Code != 0 {
		t.Fatalf("parse() exit = %v, code = %d; want exit with code 0", result.Exit, result.Code)
	}
	if stdout.Len() == 0 {
		t.Error("help output on stdout is empty")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestParseReportsSyntaxErrorsAsUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{name: "unknown flag", argv: []string{"--unknown", "example.txt"}},
		{name: "invalid duration", argv: []string{"--expire", "invalid", "example.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			result, err := parse(tt.argv, &stdout, &stderr)
			if err != nil {
				t.Fatalf("parse() error = %v", err)
			}
			if !result.Exit || result.Code != 2 {
				t.Fatalf("parse() exit = %v, code = %d; want exit with code 2", result.Exit, result.Code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if stderr.Len() == 0 {
				t.Error("usage diagnostic on stderr is empty")
			}
		})
	}
}

func TestMapArgumentsRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		args arguments
	}{
		{name: "too many files", args: arguments{Files: make([]string, share.MaxFiles+1), Expire: time.Minute}},
		{name: "zero lifetime", args: arguments{Files: []string{"one"}, Expire: 0}},
		{name: "negative lifetime", args: arguments{Files: []string{"one"}, Expire: -time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mapArguments(tt.args); err == nil {
				t.Fatal("mapArguments() error = nil, want error")
			}
		})
	}
}

func TestMapArgumentsAcceptsMultipleFiles(t *testing.T) {
	want := []string{"one", "two", "three"}
	result, err := mapArguments(arguments{Files: want, Expire: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Request.Paths, want) {
		t.Fatalf("Paths = %v, want %v", result.Request.Paths, want)
	}
}

func TestRunMapsErrorsToExitCodes(t *testing.T) {
	tests := []struct {
		name           string
		argv           []string
		wantCode       int
		wantDiagnostic string
	}{
		{
			name:           "runtime error",
			argv:           []string{"missing-file"},
			wantCode:       1,
			wantDiagnostic: "lstat shared file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if got := Run(tt.argv, &stdout, &stderr); got != tt.wantCode {
				t.Fatalf("Run() = %d, want %d", got, tt.wantCode)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.wantDiagnostic) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantDiagnostic)
			}
		})
	}
}

func TestMapArgumentsSelectsReceiveMode(t *testing.T) {
	result, err := mapArguments(arguments{
		Expire:     time.Minute,
		ReceiveDir: "/tmp/received",
	})
	if err != nil {
		t.Fatalf("mapArguments() error = %v", err)
	}

	if result.Request.Operation != app.OperationReceive {
		t.Errorf("Operation = %v, want OperationReceive",
			result.Request.Operation)
	}
	if result.Request.ReceiveDir != "/tmp/received" {
		t.Errorf("ReceiveDir = %q, want %q",
			result.Request.ReceiveDir, "/tmp/received")
	}
	if result.Request.Clipboard != "auto" {
		t.Errorf("Clipboard = %q, want auto", result.Request.Clipboard)
	}
}

func TestMapArgumentsRejectsReceiveDirInSendMode(t *testing.T) {
	_, err := mapArguments(arguments{
		Expire:     time.Minute,
		ReceiveDir: "/tmp/received",
		Files:      []string{"example.txt"},
	})
	if err == nil {
		t.Fatal("mapArguments() error = nil, want error")
	}
}

func TestReceiveDirFromHome(t *testing.T) {
	got := receiveDirFromHome("/home/alice")
	want := filepath.Join("/home/alice", "Downloads", "qshare")

	if got != want {
		t.Errorf("receiveDirFromHome() = %q, want %q", got, want)
	}
}
