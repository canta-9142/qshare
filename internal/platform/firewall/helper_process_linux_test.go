//go:build linux

package firewall

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHelperStartupFailureStopsWaiting(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "helper")
	// The finite lifetime also cleans up the process if startup regresses.
	if err := os.WriteFile(executable, []byte("#!/bin/sh\ntrap '' INT\necho diagnostic >&2\necho 'READY wrong'\nexec sleep 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := &processHelperLauncher{
		executable: func() (string, error) { return executable, nil },
		effective:  func() int { return 0 },
	}
	_, err := launcher.start(context.Background(), nixOSNFTablesBackend, testRule())
	if err == nil || !strings.Contains(err.Error(), "shutdown timed out") || !strings.Contains(err.Error(), "diagnostic") {
		t.Fatalf("start() error = %v, want shutdown timeout with diagnostics", err)
	}
}

func TestStopStartingHelper(t *testing.T) {
	for _, tt := range []struct {
		name        string
		ending      string
		wantTimeout bool
	}{
		{"graceful cleanup", "cat >/dev/null; echo cleaned >&2", false},
		{"unresponsive helper", "exec sleep 30", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", "trap '' INT; echo READY; "+tt.ending)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stderr := &lockedBuffer{}
			cmd.Stderr = stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait(); close(done) }()
			t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
			// Wait until signal handling is installed before requesting shutdown.
			if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "READY\n" {
				t.Fatalf("helper readiness = %q, %v", line, err)
			}
			err = stopStartingHelper(cmd, stdin, done)
			if tt.wantTimeout {
				if err == nil || !strings.Contains(err.Error(), "shutdown timed out") {
					t.Fatalf("stopStartingHelper() error = %v, want timeout", err)
				}
			} else if err != nil || stderr.String() != "cleaned" {
				t.Fatalf("stopStartingHelper() = %v, stderr = %q; want successful cleanup", err, stderr.String())
			}
		})
	}
}

func TestPrivilegedCommandUsesSetuidRootPKExec(t *testing.T) {
	runner := &fakeHelperRunner{paths: map[string]string{
		"pkexec": "/usr/bin/pkexec",
		"sudo":   "/usr/bin/sudo",
	}}
	launcher := &processHelperLauncher{
		runner:    runner,
		effective: func() int { return 1000 },
		stat: func(string) (os.FileInfo, error) {
			return fakePrivilegedPathInfo{name: "pkexec", mode: os.ModeSetuid | 0o755, uid: 0}, nil
		},
	}

	command, args, err := launcher.privilegedCommand("/nix/store/qshare", []string{"helper"})
	if err != nil {
		t.Fatalf("privilegedCommand() error = %v", err)
	}
	if command != "/usr/bin/pkexec" || !slices.Equal(args, []string{"/nix/store/qshare", "helper"}) {
		t.Fatalf("privilegedCommand() = %q, %v", command, args)
	}
}

func TestPrivilegedCommandFallsBackFromNonSetuidPKExecToSudo(t *testing.T) {
	runner := &fakeHelperRunner{paths: map[string]string{
		"pkexec": "/run/current-system/sw/bin/pkexec",
		"sudo":   "/run/wrappers/bin/sudo",
	}}
	launcher := &processHelperLauncher{
		runner:    runner,
		effective: func() int { return 1000 },
		stat: func(string) (os.FileInfo, error) {
			return fakePrivilegedPathInfo{name: "pkexec", mode: 0o555, uid: 0}, nil
		},
	}

	command, args, err := launcher.privilegedCommand("/nix/store/qshare", []string{"helper"})
	if err != nil {
		t.Fatalf("privilegedCommand() error = %v", err)
	}
	if command != "/run/wrappers/bin/sudo" || !slices.Equal(args, []string{"--", "/nix/store/qshare", "helper"}) {
		t.Fatalf("privilegedCommand() = %q, %v", command, args)
	}
}

func TestValidatePrivilegedExecutableAllowsRootOwnedStickyDirectory(t *testing.T) {
	const executable = "/nix/store/example-qshare/bin/qshare"
	stat := fakePrivilegedPathStat(map[string]os.FileInfo{
		executable:   fakePrivilegedPathInfo{name: "qshare", mode: 0o555, uid: 0},
		"/nix/store": fakePrivilegedPathInfo{name: "store", mode: os.ModeDir | os.ModeSticky | 0o775, uid: 0},
	})

	if err := validatePrivilegedExecutable(executable, stat); err != nil {
		t.Fatalf("validatePrivilegedExecutable() error = %v", err)
	}
}

func TestValidatePrivilegedExecutableRejectsReplaceablePath(t *testing.T) {
	const executable = "/nix/store/example-qshare/bin/qshare"
	tests := []struct {
		name     string
		override map[string]os.FileInfo
	}{
		{
			name: "writable executable",
			override: map[string]os.FileInfo{
				executable: fakePrivilegedPathInfo{name: "qshare", mode: 0o775, uid: 0},
			},
		},
		{
			name: "writable directory without sticky bit",
			override: map[string]os.FileInfo{
				"/nix/store": fakePrivilegedPathInfo{name: "store", mode: os.ModeDir | 0o775, uid: 0},
			},
		},
		{
			name: "writable regular file with sticky bit",
			override: map[string]os.FileInfo{
				executable: fakePrivilegedPathInfo{name: "qshare", mode: os.ModeSticky | 0o775, uid: 0},
			},
		},
		{
			name: "non-root owner",
			override: map[string]os.FileInfo{
				"/nix/store": fakePrivilegedPathInfo{name: "store", mode: os.ModeDir | os.ModeSticky | 0o777, uid: 1000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			override := map[string]os.FileInfo{
				executable: fakePrivilegedPathInfo{name: "qshare", mode: 0o555, uid: 0},
			}
			for path, info := range tt.override {
				override[path] = info
			}
			if err := validatePrivilegedExecutable(executable, fakePrivilegedPathStat(override)); err == nil {
				t.Fatal("validatePrivilegedExecutable() error = nil")
			}
		})
	}
}

type fakePrivilegedPathInfo struct {
	name string
	mode os.FileMode
	uid  uint32
}

func (i fakePrivilegedPathInfo) Name() string       { return i.name }
func (i fakePrivilegedPathInfo) Size() int64        { return 0 }
func (i fakePrivilegedPathInfo) Mode() os.FileMode  { return i.mode }
func (i fakePrivilegedPathInfo) ModTime() time.Time { return time.Time{} }
func (i fakePrivilegedPathInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakePrivilegedPathInfo) Sys() any           { return &syscall.Stat_t{Uid: i.uid} }

func fakePrivilegedPathStat(overrides map[string]os.FileInfo) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if info, ok := overrides[path]; ok {
			return info, nil
		}
		return fakePrivilegedPathInfo{name: path, mode: os.ModeDir | 0o755, uid: 0}, nil
	}
}

type fakeHelperRunner struct {
	paths map[string]string
}

func (r *fakeHelperRunner) lookPath(name string) (string, error) {
	path, ok := r.paths[name]
	if !ok {
		return "", exec.ErrNotFound
	}
	return path, nil
}

func (*fakeHelperRunner) run(context.Context, string, ...string) commandResult {
	return commandResult{}
}
