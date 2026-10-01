//go:build linux

package firewall

import "context"

// backend attempts to install a rule when its firewall implementation is available.
type backend interface {
	tryOpen(context.Context, Rule) (Lease, bool, error)
}

// open validates a rule and constructs the default Linux backend chain.
func open(ctx context.Context, rule Rule) (Lease, error) {
	if err := validateRule(rule); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runner := execRunner{}
	return openBackends(ctx, rule, []backend{
		&firewalld{runner: runner},
		newNixOSBackend(runner),
	})
}

// openBackends walks configured backends until one handles the rule.
func openBackends(ctx context.Context, rule Rule, backends []backend) (Lease, error) {
	for _, candidate := range backends {
		lease, handled, err := candidate.tryOpen(ctx, rule)
		if err != nil {
			return nil, err
		}
		if handled {
			return lease, nil
		}
	}
	return noopLease{}, nil
}
