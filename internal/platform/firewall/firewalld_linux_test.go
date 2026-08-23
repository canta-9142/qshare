//go:build linux

package firewall

import (
	"context"
	"errors"
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

func (r *fakeRunner) run(_ context.Context, executable string, args ...string) commandResult {
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

func TestFirewalldUnavailableIsNoop(t *testing.T) {
	runner := &fakeRunner{lookErr: exec.ErrNotFound}
	lease, err := (&firewalld{runner: runner}).open(context.Background(), testRule())
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("command calls = %v, want none", runner.calls)
	}
}

func TestStoppedFirewalldIsNoop(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{
		output:   "not running",
		exitCode: 252,
		err:      errors.New("exit status 252"),
	}}}
	lease, err := (&firewalld{runner: runner}).open(context.Background(), testRule())
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
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
	lease, err := manager.open(context.Background(), testRule())
	if err != nil {
		t.Fatalf("open() error = %v", err)
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
	lease, err := (&firewalld{runner: runner}).open(context.Background(), testRule())
	if err != nil {
		t.Fatalf("open() error = %v", err)
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

func TestFirewalldDoesNotFallBackAfterZoneQueryFailure(t *testing.T) {
	want := errors.New("dbus failed")
	runner := &fakeRunner{results: []commandResult{
		{output: "running"},
		{output: "DBUS_ERROR", exitCode: 1, err: want},
	}}
	_, err := (&firewalld{runner: runner}).open(context.Background(), testRule())
	if !errors.Is(err, want) {
		t.Fatalf("open() error = %v, want zone query error", err)
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
	lease, err := (&firewalld{runner: runner}).open(context.Background(), testRule())
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("command calls = %d, want 3", len(runner.calls))
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
	_, err := (&firewalld{runner: runner}).open(context.Background(), testRule())
	if !errors.Is(err, want) {
		t.Fatalf("open() error = %v, want authorization error", err)
	}
	if !strings.Contains(err.Error(), "AUTH_FAILED") {
		t.Fatalf("open() error = %v, want command output", err)
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
