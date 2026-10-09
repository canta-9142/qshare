//go:build linux

package firewall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// helperCleanupTimeout bounds privileged cleanup during helper shutdown.
const helperCleanupTimeout = 5 * time.Second

// helperRequest is the validated command payload accepted by the privileged helper.
type helperRequest struct {
	backend nixOSBackendKind
	rule    Rule
	expires time.Time
	leaseID string
}

// runHelperIfRequested dispatches the private helper command when requested.
func runHelperIfRequested(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) (bool, int) {
	if len(args) == 0 || args[0] != helperInvocation {
		return false, 0
	}
	if err := runHelper(args[1:], stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "qshare firewall helper: %v\n", err)
		return true, 1
	}
	return true, 0
}

// runHelper owns a privileged firewall lease until its parent exits or it expires.
func runHelper(args []string, stdin io.Reader, stdout io.Writer) error {
	if os.Geteuid() != 0 {
		return errors.New("must run with root privileges")
	}
	request, err := parseHelperRequest(args, time.Now())
	if err != nil {
		return err
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	return runHelperSession(request, stdin, stdout, execRunner{}, signals)
}

// runHelperSession monitors shutdown before invoking any firewall command.
func runHelperSession(request helperRequest, stdin io.Reader, stdout io.Writer, runner commandRunner, signals <-chan os.Signal) (runErr error) {
	deadlineCtx, cancelDeadline := context.WithDeadline(context.Background(), request.expires)
	defer cancelDeadline()
	ctx, cancel := context.WithCancel(deadlineCtx)
	defer cancel()
	inputClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		close(inputClosed)
	}()
	go func() {
		select {
		case <-inputClosed:
			cancel()
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()

	var lease Lease
	var err error
	switch request.backend {
	case nixOSNFTablesBackend:
		lease, err = openNixOSNFTables(ctx, runner, request)
	case nixOSIPTablesBackend:
		lease, err = openNixOSIPTables(ctx, runner, request)
	default:
		err = fmt.Errorf("unsupported firewall helper backend %q", request.backend)
	}
	if err != nil {
		if onlyHelperCancellation(err, ctx.Err()) {
			return nil
		}
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), helperCleanupTimeout)
		defer cancel()
		runErr = errors.Join(runErr, lease.Close(cleanupCtx))
	}()
	if ctx.Err() != nil {
		return nil
	}

	if _, err := fmt.Fprintf(stdout, "READY %s\n", request.leaseID); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

func onlyHelperCancellation(err, cancellation error) bool {
	if err == nil || cancellation == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		for _, child := range children {
			if !onlyHelperCancellation(child, cancellation) {
				return false
			}
		}
		return len(children) > 0
	}
	if wrapped := errors.Unwrap(err); wrapped != nil {
		return onlyHelperCancellation(wrapped, cancellation)
	}
	return err == cancellation
}

// parseHelperRequest converts untrusted helper arguments into a validated request.
func parseHelperRequest(args []string, now time.Time) (helperRequest, error) {
	// Treat the hidden command line as an untrusted privilege boundary. Keep
	// parsing strict before any firewall command runs.
	if len(args) != 7 {
		return helperRequest{}, errors.New("invalid firewall helper arguments")
	}
	backend := nixOSBackendKind(args[0])
	if backend != nixOSNFTablesBackend && backend != nixOSIPTablesBackend {
		return helperRequest{}, fmt.Errorf("invalid firewall helper backend %q", backend)
	}
	source, err := netip.ParsePrefix(args[2])
	if err != nil {
		return helperRequest{}, fmt.Errorf("parse firewall source: %w", err)
	}
	destination, err := netip.ParseAddr(args[3])
	if err != nil {
		return helperRequest{}, fmt.Errorf("parse firewall destination: %w", err)
	}
	port, err := strconv.ParseUint(args[4], 10, 16)
	if err != nil || port == 0 {
		return helperRequest{}, fmt.Errorf("parse firewall port %q", args[4])
	}
	expiresUnix, err := strconv.ParseInt(args[5], 10, 64)
	if err != nil {
		return helperRequest{}, fmt.Errorf("parse firewall expiration: %w", err)
	}
	expires := time.Unix(expiresUnix, 0)
	if !expires.After(now) {
		return helperRequest{}, errors.New("firewall helper expiration must be in the future")
	}
	if !validLeaseID(args[6]) {
		return helperRequest{}, errors.New("invalid firewall lease ID")
	}

	rule := Rule{
		Interface:   args[1],
		Source:      source.Masked(),
		Destination: destination,
		Port:        uint16(port),
		Timeout:     expires.Sub(now),
	}
	if err := validateRule(rule); err != nil {
		return helperRequest{}, err
	}
	return helperRequest{
		backend: backend,
		rule:    rule,
		expires: expires,
		leaseID: args[6],
	}, nil
}

// validLeaseID accepts the lowercase hexadecimal identifier generated by qshare.
func validLeaseID(value string) bool {
	if len(value) != 16 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}
