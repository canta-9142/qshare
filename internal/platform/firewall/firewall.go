package firewall

import (
	"context"
	"errors"
	"io"
	"math"
	"net/netip"
	"time"
)

// Rule describes the temporary TCP access required by one qshare session.
type Rule struct {
	Interface   string
	Source      netip.Prefix
	Destination netip.Addr
	Port        uint16
	Timeout     time.Duration
}

// Lease owns a temporary firewall rule and removes it when closed.
type Lease interface {
	Close(context.Context) error
}

// Open installs a temporary rule using the first supported firewall backend.
func Open(ctx context.Context, rule Rule) (Lease, error) {
	return open(ctx, rule)
}

// RunHelperIfRequested handles the private privileged-helper invocation.
func RunHelperIfRequested(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) (bool, int) {
	return runHelperIfRequested(args, stdin, stdout, stderr)
}

// noopLease represents a system where qshare makes no firewall changes.
type noopLease struct{}

// Close completes a no-op lease.
func (noopLease) Close(context.Context) error {
	return nil
}

// validateRule rejects rules that cannot be represented safely by every backend.
func validateRule(rule Rule) error {
	if !validInterfaceName(rule.Interface) {
		return errors.New("firewall interface has an unsupported name")
	}
	if !rule.Source.IsValid() || !rule.Source.Addr().Is4() {
		return errors.New("firewall source must be a valid IPv4 prefix")
	}
	if !rule.Destination.IsValid() || !rule.Destination.Is4() {
		return errors.New("firewall destination must be a valid IPv4 address")
	}
	if !rule.Source.Contains(rule.Destination) {
		return errors.New("firewall destination must be within the source prefix")
	}
	if rule.Port == 0 {
		return errors.New("firewall port must not be zero")
	}
	if rule.Timeout <= 0 {
		return errors.New("firewall timeout must be positive")
	}
	return nil
}

// validInterfaceName accepts Linux interface names safe for command construction.
func validInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return true
}

// timeoutSeconds rounds a duration up to a bounded positive second count.
func timeoutSeconds(timeout time.Duration) int64 {
	seconds := int64(timeout / time.Second)
	if timeout%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return min(seconds, int64(math.MaxInt32))
}
