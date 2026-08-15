package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/app"
)

func TestParseMapsArguments(t *testing.T) {
	tests := []struct {
		name         string
		argv         []string
		wantPath     string
		wantMode     app.NetworkMode
		wantLifetime time.Duration
	}{
		{
			name:         "defaults",
			argv:         []string{"example.txt"},
			wantPath:     "example.txt",
			wantMode:     app.NetworkAuto,
			wantLifetime: 10 * time.Minute,
		},
		{
			name:         "LAN and expiration",
			argv:         []string{"--lan", "--expire", "30s", "example.txt"},
			wantPath:     "example.txt",
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
			if result.Request.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", result.Request.Path, tt.wantPath)
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
		{name: "multiple files", args: arguments{Files: []string{"one", "two"}, Expire: time.Minute}},
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
