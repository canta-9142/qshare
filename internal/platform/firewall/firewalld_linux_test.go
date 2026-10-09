//go:build linux

package firewall

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

type commandCall struct {
	executable string
	args       []string
}

type fakeRunner struct {
	path      string
	lookErr   error
	results   []commandResult
	calls     []commandCall
	lookCalls int
	onRun     func(context.Context, []string)
}

func (r *fakeRunner) lookPath(string) (string, error) {
	r.lookCalls++
	if r.lookErr != nil {
		return "", r.lookErr
	}
	if r.path == "" {
		return "/usr/bin/firewall-cmd", nil
	}
	return r.path, nil
}

func (r *fakeRunner) run(ctx context.Context, executable string, args ...string) commandResult {
	if r.onRun != nil {
		r.onRun(ctx, args)
	}
	r.calls = append(r.calls, commandCall{
		executable: executable,
		args:       append([]string(nil), args...),
	})
	if len(r.results) == 0 {
		return commandResult{}
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result
}

func testRule() Rule {
	return Rule{
		Interface:   "wlan0",
		Source:      mustPrefix("192.0.2.0/24"),
		Destination: mustAddr("192.0.2.23"),
		Port:        55544,
		Timeout:     10*time.Minute + time.Nanosecond,
	}
}

func TestFirewalldUnavailableIsUnhandled(t *testing.T) {
	runner := &fakeRunner{lookErr: exec.ErrNotFound}
	lease, handled, err := (&firewalld{runner: runner}).tryOpen(context.Background(), testRule())
	if handled {
		t.Fatal("tryOpen() handled = true, want false")
	}
	if lease != nil {
		t.Fatalf("tryOpen() lease = %v, want nil", lease)
	}
	if err != nil {
		t.Fatalf("tryOpen() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("command calls = %v, want none", runner.calls)
	}
}

func TestStoppedFirewalldIsUnhandled(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{
		output:   "not running",
		exitCode: 252,
		err:      errors.New("exit status 252"),
	}}}
	lease, handled, err := (&firewalld{runner: runner}).tryOpen(context.Background(), testRule())
	if handled {
		t.Fatal("tryOpen() handled = true, want false")
	}
	if lease != nil {
		t.Fatalf("tryOpen() lease = %v, want nil", lease)
	}
	if err != nil {
		t.Fatalf("tryOpen() error = %v", err)
	}
	if len(runner.calls) != 1 || !slices.Equal(runner.calls[0].args, []string{"--state"}) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestFirewalldAddsAndRemovesScopedTemporaryRule(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{
		{output: "running"},
		{output: "home"},
		{output: "no", exitCode: 1, err: errors.New("exit status 1")},
		{},
		{},
	}}
	manager := &firewalld{runner: runner}
	lease, handled, err := manager.tryOpen(context.Background(), testRule())
	if !handled {
		t.Fatal("tryOpen() handled = false, want true")
	}
	if err != nil {
		t.Fatalf("tryOpen() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if len(runner.calls) != 5 {
		t.Fatalf("command calls = %d, want 5: %#v", len(runner.calls), runner.calls)
	}
	addArgs := runner.calls[3].args
	if !slices.Contains(addArgs, "--zone=home") {
		t.Errorf("add args = %v, want home zone", addArgs)
	}
	if !slices.Contains(addArgs, "--timeout=601s") {
		t.Errorf("add args = %v, want rounded-up timeout", addArgs)
	}
	richRule := `rule family="ipv4" source address="192.0.2.0/24" destination address="192.0.2.23" port port="55544" protocol="tcp" accept`
	if !slices.Contains(addArgs, "--add-rich-rule="+richRule) {
		t.Errorf("add args = %v, want scoped rich rule", addArgs)
	}
	if !slices.Contains(runner.calls[4].args, "--remove-rich-rule="+richRule) {
		t.Errorf("remove args = %v, want matching rich rule", runner.calls[4].args)
	}
}

func TestFirewalldUsesDefaultZoneForUnboundInterface(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{
		{output: "running"},
		{output: "no zone"},
		{output: "public"},
		{output: "yes"},
	}}
	lease, handled, err := (&firewalld{runner: runner}).tryOpen(context.Background(), testRule())
	if !handled {
		t.Fatal("tryOpen() handled = false, want true")
	}
	if err != nil {
		t.Fatalf("tryOpen() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("command calls = %d, want 4", len(runner.calls))
	}
	if !slices.Contains(runner.calls[3].args, "--zone=public") {
		t.Fatalf("query args = %v, want public zone", runner.calls[3].args)
	}
}

func TestFirewalldDefaultZoneFailurePreservesCancellation(t *testing.T) {
	for _, reason := range []string{"cancel", "expire", "command failure"} {
		t.Run(reason, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			if reason == "expire" {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
			}
			defer cancel()
			commandErr := errors.New("firewall-cmd failed")
			runner := &fakeRunner{
				results: []commandResult{
					{output: "running"},
					{output: "no zone"},
					{err: commandErr, exitCode: -1},
				},
				onRun: func(ctx context.Context, args []string) {
					if !slices.Equal(args, []string{"--get-default-zone"}) || reason == "command failure" {
						return
					}
					if reason == "cancel" {
						cancel()
					}
					<-ctx.Done()
				},
			}
			lease, handled, err := (&firewalld{runner: runner}).tryOpen(ctx, testRule())
			want := commandErr
			if reason == "cancel" {
				want = context.Canceled
			} else if reason == "expire" {
				want = context.DeadlineExceeded
			}
			if lease != nil || !handled || !errors.Is(err, want) || len(runner.calls) != 3 {
				t.Fatalf("lease=%v, handled=%t, error=%v, calls=%v", lease, handled, err, runner.calls)
			}
		})
	}
}

func TestFirewalldDoesNotFallBackAfterZoneQueryFailure(t *testing.T) {
	want := errors.New("dbus failed")
	runner := &fakeRunner{results: []commandResult{
		{output: "running"},
		{output: "DBUS_ERROR", exitCode: 1, err: want},
	}}
	_, handled, err := (&firewalld{runner: runner}).tryOpen(context.Background(), testRule())
	if !handled {
		t.Fatal("tryOpen() handled = false, want true")
	}
	if !errors.Is(err, want) {
		t.Fatalf("tryOpen() error = %v, want zone query error", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("command calls = %d, want 2", len(runner.calls))
	}
}

func TestFirewalldDoesNotRemovePreexistingRule(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{
		{output: "running"},
		{output: "home"},
		{output: "yes"},
	}}
	lease, handled, err := (&firewalld{runner: runner}).tryOpen(context.Background(), testRule())
	if !handled {
		t.Fatal("tryOpen() handled = false, want true")
	}
	if err != nil {
		t.Fatalf("tryOpen() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("command calls = %d, want 3", len(runner.calls))
	}
}

func TestFirewalldAddOwnershipAndCancellation(t *testing.T) {
	cleanupFailure := errors.New("rule removal failed")
	for _, tt := range []struct {
		name           string
		cancel         bool
		alreadyEnabled bool
		cleanupFailure bool
	}{
		{"new rule", false, false, false},
		{"canceled new rule", true, false, false},
		{"concurrent rule", false, true, false},
		{"canceled concurrent rule", true, true, false},
		{"canceled cleanup failure", true, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			addResult := commandResult{output: "success"}
			if tt.alreadyEnabled {
				addResult.output = "Warning: ALREADY_ENABLED: rule already enabled"
			}
			removeResult := commandResult{}
			if tt.cleanupFailure {
				removeResult = commandResult{err: cleanupFailure, exitCode: 1}
			}
			removed := false
			runner := &fakeRunner{
				results: []commandResult{
					{output: "running"}, {output: "home"},
					{output: "no", exitCode: 1, err: errors.New("not enabled")},
					addResult, removeResult, {output: "running"}, {output: "yes"},
				},
				onRun: func(commandCtx context.Context, args []string) {
					for _, arg := range args {
						if strings.HasPrefix(arg, "--add-rich-rule=") {
							if tt.cancel {
								cancel()
							}
							if commandCtx.Err() != nil {
								t.Fatalf("insertion stopped before reporting ownership: %v", commandCtx.Err())
							}
						}
						if strings.HasPrefix(arg, "--remove-rich-rule=") {
							removed = true
							assertFirewallCleanupContext(t, commandCtx)
						}
					}
				},
			}
			lease, handled, err := (&firewalld{runner: runner}).tryOpen(ctx, testRule())
			if !handled {
				t.Fatal("firewalld did not handle the rule")
			}
			if tt.cancel {
				if lease != nil || !errors.Is(err, context.Canceled) {
					t.Fatalf("lease=%v, cancellation error=%v", lease, err)
				}
			} else {
				if err != nil || lease == nil {
					t.Fatalf("lease=%v, error=%v", lease, err)
				}
				cleanupCtx, cancel := context.WithTimeout(t.Context(), helperCleanupTimeout)
				defer cancel()
				err = lease.Close(cleanupCtx)
			}
			if removed == tt.alreadyEnabled || errors.Is(err, cleanupFailure) != tt.cleanupFailure {
				t.Fatalf("removed=%t, error=%v", removed, err)
			}
		})
	}
}

func TestFirewalldAuthenticationWaitAndCancellationGrace(t *testing.T) {
	for _, cancelStartup := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelStartup), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*helperCleanupTimeout)
			defer cancel()
			commandFailure := errors.New("insertion stopped")
			addResult := commandResult{output: "success"}
			if cancelStartup {
				addResult = commandResult{err: commandFailure, exitCode: -1}
			}
			runner := &fakeRunner{
				results: []commandResult{
					{output: "running"}, {output: "home"},
					{output: "no", exitCode: 1, err: errors.New("not enabled")}, addResult,
				},
				onRun: func(commandCtx context.Context, args []string) {
					if len(args) != 3 || !strings.HasPrefix(args[1], "--add-rich-rule=") {
						return
					}
					if cancelStartup {
						cancel()
					}
					timer := time.NewTimer(helperCleanupTimeout + time.Second)
					defer timer.Stop()
					select {
					case <-commandCtx.Done():
						if !cancelStartup || !errors.Is(context.Cause(commandCtx), context.DeadlineExceeded) {
							t.Fatalf("unexpected insertion cancellation: %v", context.Cause(commandCtx))
						}
					case <-timer.C:
						if cancelStartup {
							t.Fatal("insertion did not stop after cancellation grace")
						}
					}
				},
			}
			lease, handled, err := (&firewalld{runner: runner}).tryOpen(ctx, testRule())
			if !handled || len(runner.calls) != 4 {
				t.Fatalf("handled=%t, commands=%v", handled, runner.calls)
			}
			if cancelStartup {
				if lease != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, commandFailure) {
					t.Fatalf("missing cancellation, grace timeout, or command failure: lease=%v, error=%v", lease, err)
				}
			} else if err != nil || lease == nil {
				t.Fatalf("authentication wait failed: lease=%v, error=%v", lease, err)
			}
		})
	}
}

func assertFirewallCleanupContext(t *testing.T, ctx context.Context) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if ctx.Err() != nil || !ok || time.Until(deadline) <= 0 || time.Until(deadline) > helperCleanupTimeout {
		t.Errorf("missing independent bounded context: error=%v, deadline=%v", ctx.Err(), deadline)
	}
}

func TestFirewalldReportsAddFailure(t *testing.T) {
	want := errors.New("authorization denied")
	runner := &fakeRunner{results: []commandResult{
		{output: "running"},
		{output: "home"},
		{output: "no", exitCode: 1, err: errors.New("exit status 1")},
		{output: "AUTH_FAILED", exitCode: 1, err: want},
		{output: "no", exitCode: 1, err: errors.New("exit status 1")},
	}}
	_, handled, err := (&firewalld{runner: runner}).tryOpen(context.Background(), testRule())
	if !handled {
		t.Fatal("tryOpen() handled = false, want true")
	}
	if !errors.Is(err, want) {
		t.Fatalf("tryOpen() error = %v, want authorization error", err)
	}
	if !strings.Contains(err.Error(), "AUTH_FAILED") {
		t.Fatalf("tryOpen() error = %v, want command output", err)
	}
}

func TestFirewalldCloseAcceptsAlreadyExpiredRule(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{
		{output: "NOT_ENABLED", exitCode: 1, err: errors.New("exit status 1")},
		{output: "running"},
		{output: "no", exitCode: 1, err: errors.New("exit status 1")},
	}}
	lease := &firewalldLease{
		manager: &firewalld{runner: runner},
		zone:    "home",
		rule:    "rule",
		owned:   true,
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestValidateRule(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Rule)
	}{
		{name: "empty interface", mutate: func(rule *Rule) { rule.Interface = "" }},
		{name: "unsafe interface", mutate: func(rule *Rule) { rule.Interface = `wlan0 accept` }},
		{name: "long interface", mutate: func(rule *Rule) { rule.Interface = "interface-name-16" }},
		{name: "invalid source", mutate: func(rule *Rule) { rule.Source = mustPrefix("2001:db8::/64") }},
		{name: "invalid destination", mutate: func(rule *Rule) { rule.Destination = mustAddr("2001:db8::1") }},
		{name: "destination outside source", mutate: func(rule *Rule) { rule.Destination = mustAddr("198.51.100.1") }},
		{name: "zero port", mutate: func(rule *Rule) { rule.Port = 0 }},
		{name: "zero timeout", mutate: func(rule *Rule) { rule.Timeout = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := testRule()
			tt.mutate(&rule)
			if err := validateRule(rule); err == nil {
				t.Fatal("validateRule() error = nil")
			}
		})
	}
}

func mustAddr(value string) netip.Addr {
	return netip.MustParseAddr(value)
}

func mustPrefix(value string) netip.Prefix {
	return netip.MustParsePrefix(value)
}
