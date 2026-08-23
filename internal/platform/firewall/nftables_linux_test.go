//go:build linux

package firewall

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNFTRuleHandle(t *testing.T) {
	output := `{"nftables":[{"add":{"rule":{"family":"inet","table":"nixos-fw","chain":"input-allow","comment":"qshare:0123456789abcdef","handle":42}}}]}`
	handle, ok := nftRuleHandle(output, "qshare:0123456789abcdef")
	if !ok || handle != 42 {
		t.Fatalf("nftRuleHandle() = %d, %v; want 42, true", handle, ok)
	}
	if _, ok := nftRuleHandle(output, "qshare:other"); ok {
		t.Fatal("nftRuleHandle() found unrelated rule")
	}
}

func TestOpenNixOSNFTablesAddsAndRemovesOwnedRule(t *testing.T) {
	request := helperRequest{
		backend: nixOSNFTablesBackend,
		rule:    testRule(),
		expires: time.Now().Add(10 * time.Minute),
		leaseID: "0123456789abcdef",
	}
	runner := &fakeRunner{path: "/usr/bin/nft", results: []commandResult{
		{},
		{},
		{output: `{"nftables":[{"add":{"rule":{"comment":"qshare:0123456789abcdef","handle":42}}}]}`},
		{},
		{},
	}}
	lease, err := openNixOSNFTables(context.Background(), runner, request)
	if err != nil {
		t.Fatalf("openNixOSNFTables() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(runner.calls) != 5 {
		t.Fatalf("command calls = %d, want 5: %#v", len(runner.calls), runner.calls)
	}
	if !slices.Equal(runner.calls[0].args, []string{
		"add", "set", "inet", "nixos-fw", "qshare_0123456789abcdef",
		"{", "type", "ipv4_addr", ";", "flags", "interval,timeout", ";", "}",
	}) {
		t.Errorf("add set args = %v", runner.calls[0].args)
	}
	addRule := runner.calls[2].args
	for _, want := range []string{"input-allow", "wlan0", "@qshare_0123456789abcdef", "192.0.2.23", "55544", `"qshare:0123456789abcdef"`} {
		if !slices.Contains(addRule, want) {
			t.Errorf("add rule args = %v, missing %q", addRule, want)
		}
	}
	if !slices.Equal(runner.calls[3].args, []string{
		"delete", "rule", "inet", "nixos-fw", "input-allow", "handle", "42",
	}) {
		t.Errorf("delete rule args = %v", runner.calls[3].args)
	}
	if !slices.Equal(runner.calls[4].args, []string{
		"delete", "set", "inet", "nixos-fw", "qshare_0123456789abcdef",
	}) {
		t.Errorf("delete set args = %v", runner.calls[4].args)
	}
}

func TestOpenNixOSNFTablesFailsClosedWithoutHandle(t *testing.T) {
	runner := &fakeRunner{path: "/usr/bin/nft", results: []commandResult{{}, {}, {output: `{}`}, {}}}
	request := helperRequest{rule: testRule(), expires: time.Now().Add(time.Minute), leaseID: "0123456789abcdef"}
	_, err := openNixOSNFTables(context.Background(), runner, request)
	if err == nil || !strings.Contains(err.Error(), "handle") {
		t.Fatalf("openNixOSNFTables() error = %v", err)
	}
	if len(runner.calls) != 4 || !slices.Equal(runner.calls[3].args, []string{
		"flush", "set", "inet", "nixos-fw", "qshare_0123456789abcdef",
	}) {
		t.Fatalf("cleanup call = %#v", runner.calls)
	}
}
