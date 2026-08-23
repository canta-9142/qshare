//go:build linux

package firewall

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestOpenNixOSIPTablesAddsExpiringScopedRule(t *testing.T) {
	expires := time.Date(2026, 8, 23, 12, 10, 0, 0, time.UTC)
	request := helperRequest{rule: testRule(), expires: expires, leaseID: "0123456789abcdef"}
	runner := &fakeRunner{path: "/usr/bin/iptables", results: []commandResult{{}, {}}}
	lease, err := openNixOSIPTables(context.Background(), runner, request)
	if err != nil {
		t.Fatalf("openNixOSIPTables() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("command calls = %d, want 2", len(runner.calls))
	}
	wantRule := []string{
		"-i", "wlan0", "-s", "192.0.2.0/24", "-d", "192.0.2.23",
		"-p", "tcp", "--dport", "55544", "-m", "time",
		"--datestop", "2026-08-23T12:10:00", "-m", "comment",
		"--comment", "qshare:0123456789abcdef", "-j", "nixos-fw-accept",
	}
	if !slices.Equal(runner.calls[0].args, append([]string{"-w", "-I", "nixos-fw", "1"}, wantRule...)) {
		t.Errorf("insert args = %v", runner.calls[0].args)
	}
	if !slices.Equal(runner.calls[1].args, append([]string{"-w", "-D", "nixos-fw"}, wantRule...)) {
		t.Errorf("delete args = %v", runner.calls[1].args)
	}
}

func TestIPTablesCloseAcceptsExpiredOrRemovedRule(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{
		{err: errors.New("not found"), exitCode: 1},
		{err: errors.New("not found"), exitCode: 1},
	}}
	lease := &iptablesLease{runner: runner, executable: "iptables", ruleArgs: []string{"rule"}}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
