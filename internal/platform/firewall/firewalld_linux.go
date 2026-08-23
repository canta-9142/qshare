//go:build linux

package firewall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// commandResult captures the relevant outcome of an external command.
type commandResult struct {
	output   string
	exitCode int
	err      error
}

// commandRunner abstracts executable discovery and command execution.
type commandRunner interface {
	lookPath(string) (string, error)
	run(context.Context, string, ...string) commandResult
}

// execRunner executes firewall tools as child processes.
type execRunner struct{}

// lookPath resolves an executable through PATH.
func (execRunner) lookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// run executes a command and captures its combined output and exit status.
func (execRunner) run(ctx context.Context, executable string, args ...string) commandResult {
	output, err := exec.CommandContext(ctx, executable, args...).CombinedOutput()
	result := commandResult{
		output:   strings.TrimSpace(string(output)),
		exitCode: 0,
		err:      err,
	}
	if err == nil {
		return result
	}

	result.exitCode = -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
	}
	return result
}

// firewalld manages runtime rich rules through firewall-cmd.
type firewalld struct {
	runner commandRunner
}

// firewalldLease tracks ownership of one firewalld rich rule.
type firewalldLease struct {
	manager *firewalld
	zone    string
	rule    string
	owned   bool

	once sync.Once
	err  error
}

// open preserves the standalone firewalld behavior used by focused tests.
func (f *firewalld) open(ctx context.Context, rule Rule) (Lease, error) {
	lease, handled, err := f.tryOpen(ctx, rule)
	if err != nil || handled {
		return lease, err
	}
	return noopLease{}, nil
}

// tryOpen installs a rich rule when firewalld is present and running.
func (f *firewalld) tryOpen(ctx context.Context, rule Rule) (Lease, bool, error) {
	executable, err := f.runner.lookPath("firewall-cmd")
	if errors.Is(err, exec.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find firewall-cmd: %w", err)
	}

	state := f.runner.run(ctx, executable, "--state")
	if state.err != nil {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if state.output == "not running" {
			return nil, false, nil
		}
		return nil, false, commandError("check firewalld state", state)
	}
	if state.output != "running" {
		return nil, false, fmt.Errorf("unexpected firewalld state %q", state.output)
	}

	zone, err := f.zoneForInterface(ctx, executable, rule.Interface)
	if err != nil {
		return nil, true, err
	}

	richRule := fmt.Sprintf(
		`rule family="ipv4" source address="%s" destination address="%s" port port="%d" protocol="tcp" accept`,
		rule.Source.Masked(),
		rule.Destination,
		rule.Port,
	)

	exists, err := f.queryRule(ctx, executable, zone, richRule)
	if err != nil {
		return nil, true, err
	}
	if exists {
		return &firewalldLease{manager: f, zone: zone, rule: richRule}, true, nil
	}

	seconds := timeoutSeconds(rule.Timeout)
	result := f.runner.run(
		ctx,
		executable,
		"--zone="+zone,
		"--add-rich-rule="+richRule,
		"--timeout="+strconv.FormatInt(seconds, 10)+"s",
	)
	if result.err != nil {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}

		// Another process may have installed the same rule between the query
		// and add operations. In that case it owns the rule and qshare must not
		// remove it.
		exists, queryErr := f.queryRule(ctx, executable, zone, richRule)
		if queryErr == nil && exists {
			return &firewalldLease{manager: f, zone: zone, rule: richRule}, true, nil
		}
		return nil, true, commandError("add temporary firewalld rule", result)
	}

	return &firewalldLease{
		manager: f,
		zone:    zone,
		rule:    richRule,
		owned:   true,
	}, true, nil
}

// zoneForInterface resolves an interface zone or falls back to the default zone.
func (f *firewalld) zoneForInterface(ctx context.Context, executable, iface string) (string, error) {
	result := f.runner.run(ctx, executable, "--get-zone-of-interface="+iface)
	if result.err == nil {
		if result.output != "" && result.output != "no zone" {
			return result.output, nil
		}
	} else {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", commandError("determine firewalld zone for interface", result)
	}

	result = f.runner.run(ctx, executable, "--get-default-zone")
	if result.err != nil {
		return "", commandError("determine default firewalld zone", result)
	}
	if result.output == "" {
		return "", errors.New("firewalld returned an empty default zone")
	}
	return result.output, nil
}

// queryRule reports whether an exact rich rule exists in a zone.
func (f *firewalld) queryRule(ctx context.Context, executable, zone, rule string) (bool, error) {
	result := f.runner.run(
		ctx,
		executable,
		"--zone="+zone,
		"--query-rich-rule="+rule,
	)
	if result.err == nil {
		return result.output == "yes", nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if result.exitCode == 1 && result.output == "no" {
		return false, nil
	}
	return false, commandError("query firewalld rule", result)
}

// Close removes the rich rule only when this lease created it.
func (l *firewalldLease) Close(ctx context.Context) error {
	l.once.Do(func() {
		if !l.owned {
			return
		}

		executable, err := l.manager.runner.lookPath("firewall-cmd")
		if errors.Is(err, exec.ErrNotFound) {
			return
		}
		if err != nil {
			l.err = fmt.Errorf("find firewall-cmd during cleanup: %w", err)
			return
		}

		result := l.manager.runner.run(
			ctx,
			executable,
			"--zone="+l.zone,
			"--remove-rich-rule="+l.rule,
		)
		if result.err == nil {
			return
		}
		if err := ctx.Err(); err != nil {
			l.err = err
			return
		}

		state := l.manager.runner.run(ctx, executable, "--state")
		if state.output == "not running" {
			return
		}

		exists, queryErr := l.manager.queryRule(ctx, executable, l.zone, l.rule)
		if queryErr == nil && !exists {
			return
		}
		l.err = commandError("remove temporary firewalld rule", result)
	})
	return l.err
}

// commandError adds command diagnostics to an operation error.
func commandError(operation string, result commandResult) error {
	if result.output == "" {
		return fmt.Errorf("%s: %w", operation, result.err)
	}
	return fmt.Errorf("%s: %w: %s", operation, result.err, result.output)
}
