//go:build linux

package firewall

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// helperInvocation identifies the private privileged-helper command.
const helperInvocation = "__qshare_firewall_helper"

// processHelperLauncher starts and monitors a privileged qshare subprocess.
type processHelperLauncher struct {
	runner     commandRunner
	executable func() (string, error)
	effective  func() int
	stat       func(string) (os.FileInfo, error)
}

// newProcessHelperLauncher constructs a launcher with production OS dependencies.
func newProcessHelperLauncher(runner commandRunner) *processHelperLauncher {
	return &processHelperLauncher{
		runner:     runner,
		executable: os.Executable,
		effective:  os.Geteuid,
		stat:       os.Stat,
	}
}

// start launches a helper and waits for confirmation that its rule is active.
func (l *processHelperLauncher) start(
	ctx context.Context,
	kind nixOSBackendKind,
	rule Rule,
) (Lease, error) {
	self, err := l.executable()
	if err != nil {
		return nil, fmt.Errorf("locate qshare executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return nil, fmt.Errorf("resolve qshare executable: %w", err)
	}
	if l.effective() != 0 {
		if err := validatePrivilegedExecutable(self, l.stat); err != nil {
			return nil, err
		}
	}

	leaseID, err := newLeaseID()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(time.Duration(timeoutSeconds(rule.Timeout)) * time.Second)
	helperArgs := []string{
		helperInvocation,
		string(kind),
		rule.Interface,
		rule.Source.Masked().String(),
		rule.Destination.String(),
		strconv.FormatUint(uint64(rule.Port), 10),
		strconv.FormatInt(expiresAt.Unix(), 10),
		leaseID,
	}

	command, commandArgs, err := l.privilegedCommand(self, helperArgs)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command, commandArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create firewall helper input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create firewall helper output: %w", err)
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start privileged firewall helper: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		ready <- strings.TrimSpace(line)
	}()

	wantReady := "READY " + leaseID
	select {
	case line := <-ready:
		if line != wantReady {
			_ = stdin.Close()
			err := <-done
			return nil, helperProcessError("firewall helper did not become ready", err, stderr.String())
		}
		return &processLease{stdin: stdin, done: done, stderr: stderr}, nil

	case err := <-done:
		return nil, helperProcessError("firewall helper exited before becoming ready", err, stderr.String())

	case <-ctx.Done():
		_ = stdin.Close()
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = cmd.Process.Kill()
		}
		return nil, ctx.Err()
	}
}

// privilegedCommand selects direct execution, pkexec, or sudo as appropriate.
func (l *processHelperLauncher) privilegedCommand(self string, helperArgs []string) (string, []string, error) {
	if l.effective() == 0 {
		return self, helperArgs, nil
	}
	if pkexec, err := l.runner.lookPath("pkexec"); err == nil {
		return pkexec, append([]string{self}, helperArgs...), nil
	}
	if sudo, err := l.runner.lookPath("sudo"); err == nil {
		args := []string{"--", self}
		return sudo, append(args, helperArgs...), nil
	}
	return "", nil, errors.New("automatic NixOS firewall configuration requires pkexec or sudo")
}

// processLease keeps the helper alive through an open stdin pipe.
type processLease struct {
	stdin  io.WriteCloser
	done   <-chan error
	stderr *lockedBuffer

	once sync.Once
	err  error
}

// Close signals helper cleanup by closing its stdin and waiting for exit.
func (l *processLease) Close(ctx context.Context) error {
	l.once.Do(func() {
		if err := l.stdin.Close(); err != nil {
			l.err = fmt.Errorf("close firewall helper input: %w", err)
			return
		}
		select {
		case err := <-l.done:
			if err != nil {
				l.err = helperProcessError("firewall helper cleanup failed", err, l.stderr.String())
			}
		case <-ctx.Done():
			l.err = ctx.Err()
		}
	})
	return l.err
}

// newLeaseID generates a random identifier used to own backend resources.
func newLeaseID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate firewall lease ID: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// validatePrivilegedExecutable rejects an executable path replaceable by non-root users.
func validatePrivilegedExecutable(path string, stat func(string) (os.FileInfo, error)) error {
	current := path
	for {
		info, err := stat(current)
		if err != nil {
			return fmt.Errorf("inspect privileged executable path %q: %w", current, err)
		}
		owner, ok := info.Sys().(*syscall.Stat_t)
		writable := info.Mode().Perm()&0o022 != 0
		// In a sticky directory, only the entry owner, directory owner, or root
		// can remove or rename an entry. Root ownership therefore keeps the next
		// validated path component non-replaceable, as it does in /nix/store.
		stickyDirectory := info.IsDir() && info.Mode()&os.ModeSticky != 0
		if !ok || owner.Uid != 0 || (writable && !stickyDirectory) {
			return fmt.Errorf("refusing to elevate writable qshare path %q; install qshare in a root-owned location", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

// helperProcessError combines a process failure with captured diagnostics.
func helperProcessError(operation string, err error, diagnostic string) error {
	if diagnostic == "" {
		if err == nil {
			return errors.New(operation)
		}
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err == nil {
		return fmt.Errorf("%s: %s", operation, diagnostic)
	}
	return fmt.Errorf("%s: %w: %s", operation, err, diagnostic)
}

// lockedBuffer safely captures helper diagnostics while another goroutine waits.
type lockedBuffer struct {
	mu   sync.Mutex
	data strings.Builder
}

// Write appends bytes while holding the buffer lock.
func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(data)
}

// String returns trimmed buffered diagnostics under the buffer lock.
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.data.String())
}
