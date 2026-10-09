//go:build linux

package firewall

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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
	if err := os.WriteFile(executable, []byte("#!/bin/sh\ntrap '' INT\necho diagnostic >&2\necho 'READY wrong'\nexec sleep 10\n"), 0o700); err != nil {
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
		trap        string
		ending      string
		wantTimeout bool
	}{
		{"graceful cleanup", "trap '' INT", "cat >/dev/null; echo cleaned >&2", false},
		{"slow cleanup", "trap '' INT", "cat >/dev/null; sleep 2; echo cleaned >&2", false},
		{"SIGINT", "trap - INT", "exec sleep 30", false},
		{"SIGINT exit status", "trap '' INT", "cat >/dev/null; exit 130", false},
		{"unresponsive helper", "trap '' INT", "exec sleep 30", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", tt.trap+"; echo READY; "+tt.ending)
			if tt.name == "SIGINT" {
				cmd = exec.Command(os.Args[0], "-test.run=^TestInterruptibleHelperProcess$")
				cmd.Env = append(os.Environ(), "QSHARE_TEST_INTERRUPT_HELPER=1")
			}
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
			} else if err != nil || (strings.HasSuffix(tt.name, "cleanup") && stderr.String() != "cleaned") {
				t.Fatalf("stopStartingHelper() = %v, stderr = %q; want successful cleanup", err, stderr.String())
			}
			if tt.name == "SIGINT" {
				status := cmd.ProcessState.Sys().(syscall.WaitStatus)
				if !status.Signaled() || status.Signal() != syscall.SIGINT {
					t.Fatalf("helper status = %v, want SIGINT", status)
				}
			}
		})
	}
}

func TestInterruptibleHelperProcess(t *testing.T) {
	if os.Getenv("QSHARE_TEST_INTERRUPT_HELPER") != "1" {
		return
	}
	signal.Reset(os.Interrupt)
	fmt.Fprintln(os.Stdout, "READY")
	time.Sleep(30 * time.Second)
}

func TestHelperStartupCancellationPreservesCleanupFailures(t *testing.T) {
	for _, tt := range []struct {
		name       string
		ending     string
		wantDetail string
	}{
		{"graceful cleanup", "cat >/dev/null", ""},
		{"cleanup failure", "cat >/dev/null; echo cleanup-failed >&2; exit 1", "cleanup-failed"},
		{"shutdown timeout", "exec sleep 10", "shutdown timed out"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "helper")
			script := "#!/bin/sh\ntrap '' INT\n: > \"$0.started\"\n" + tt.ending + "\n"
			if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			launcher := &processHelperLauncher{
				executable: func() (string, error) { return executable, nil },
				effective:  func() int { return 0 },
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := launcher.start(ctx, nixOSNFTablesBackend, testRule())
				result <- err
			}()
			// Cancel only once the helper has installed its signal handling.
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			started := time.NewTimer(2 * time.Second)
			defer started.Stop()
			for {
				if _, err := os.Stat(executable + ".started"); err == nil {
					break
				}
				select {
				case <-ticker.C:
				case err := <-result:
					t.Fatalf("helper exited before cancellation: %v", err)
				case <-started.C:
					t.Fatal("helper did not start")
				}
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation was lost: %v", err)
				}
				if tt.wantDetail == "" {
					if err != context.Canceled {
						t.Fatalf("graceful cancellation error = %v", err)
					}
				} else if !strings.Contains(err.Error(), tt.wantDetail) {
					t.Fatalf("cleanup failure was lost: %v", err)
				}
				if tt.name == "cleanup failure" {
					var exitErr *exec.ExitError
					if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
						t.Fatalf("helper exit status was lost: %v", err)
					}
				}
			case <-time.After(helperCleanupTimeout + 3*time.Second):
				t.Fatal("helper cancellation did not finish")
			}
		})
	}
}

// Done releases the child only after cancellation, then lets EOF and exit
// notifications become ready alongside cancellation in the launcher's select.
type simultaneousHelperExitContext struct {
	context.Context
	release func()
}

func (c simultaneousHelperExitContext) Done() <-chan struct{} {
	c.release()
	return c.Context.Done()
}

func TestHelperSimultaneousExitAndCancellation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		ending  string
		expire  bool
		failure bool
	}{
		{"graceful cancel", "exit 0", false, false},
		{"graceful expiry", "exit 0", true, false},
		{"SIGINT exit status", "exit 130", false, false},
		{"cleanup failure", "echo cleanup-failed >&2; exit 1", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for range 24 {
				executable := filepath.Join(t.TempDir(), "helper")
				script := "#!/bin/sh\ntrap '' INT\nwhile [ ! -f \"$0.cancel\" ]; do sleep 0.001; done\n" + tt.ending + "\n"
				if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				launcher := &processHelperLauncher{
					executable: func() (string, error) { return executable, nil },
					effective:  func() int { return 0 },
				}
				var ctx context.Context
				var cancel context.CancelFunc
				if tt.expire {
					ctx, cancel = context.WithTimeout(t.Context(), 10*time.Millisecond)
				} else {
					ctx, cancel = context.WithCancel(t.Context())
				}
				canceledCtx := simultaneousHelperExitContext{Context: ctx, release: func() {
					if !tt.expire {
						cancel()
					}
					<-ctx.Done()
					if err := os.WriteFile(executable+".cancel", nil, 0o600); err != nil {
						t.Fatal(err)
					}
					time.Sleep(30 * time.Millisecond)
				}}
				lease, err := launcher.start(canceledCtx, nixOSNFTablesBackend, testRule())
				cancel()
				if lease != nil || !errors.Is(err, ctx.Err()) {
					t.Fatalf("cancellation was lost: lease=%v, error=%v", lease, err)
				}
				if !tt.failure {
					if !onlyHelperCancellation(err, ctx.Err()) {
						t.Fatalf("successful shutdown reported a failure: %v", err)
					}
					continue
				}
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || !strings.Contains(err.Error(), "cleanup-failed") {
					t.Fatalf("cleanup failure or diagnostics were lost: %v", err)
				}
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
